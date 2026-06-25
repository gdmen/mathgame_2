# Adaptive difficulty & progression

How the system moves a kid through the difficulty band: the global work-load adjuster, per-topic
difficulty stats, the serving lottery that weights weak/thin topics, and the spaced-repetition
review queue. The *content* of the difficulty band (the formula, the bits, the ceiling) is owned by
**`docs/problem-generation.md`**; this doc owns the *levers that move within it*.

**If you change any behavior described here, update this document in the same PR.** Mechanical
enforcement: the doc-sync test (`server/api/docs_sync_test.go`, `TestDocsSyncAdaptiveDifficulty`)
fails CI when the anchors below disagree with the code.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
spaced_rep_intervals: 1, 3, 7
max_target: 20
min_target_difficulty: 3.0
problem_selection_epsilon: 1.5
```
<!-- END DOC-SYNC ANCHORS -->

## The model

Three persisted levers move a kid through the band, all bounded above by the envelope ceiling
`MaxDiffForBitmap` (owned by problem-generation.md):

| Lever | Stored in | Adjusted by | Bounds |
|---|---|---|---|
| `settings.target_difficulty` | `settings` | global work-load adjuster | `[MinTargetDifficulty, MaxDiffForBitmap]` |
| `gamestate.target` (problems per session) | `gamestates` | global work-load adjuster | `[minProbs, maxTarget]` |
| per-topic `target_difficulty` | `topic_stats` | `adjustTopicDifficulty`, per accuracy window | floored at `minDiff` |

The two difficulty levers feed **selection** (problem-generation.md): the serving lottery picks a
topic, then selection draws within `± problemSelectionEpsilon` of that topic's difficulty (or the
global target when the topic has no row).

Adjustment math is shared between the global and per-topic adjusters but the *signal* differs:
global moves on **work percentage** (time-on-task), topic moves on **accuracy**. The step rule is
identical in both: at least a full point, otherwise `diffIncrease` (5%) of the current difficulty —
so low difficulties move by whole points and high ones move proportionally.

## Reference table — adjustment levers

All defined as locals/consts at their use site; cite the enclosing symbol.

| Lever | Value | Where | Meaning |
|---|---|---|---|
| `maxTarget` | 20 | `process_events.go` const | ceiling on problems-per-session |
| `minProbs` | 5 | `processEvent`, `DONE_WATCHING_VIDEO` | floor on problems-per-session |
| `epsilon` | 0.05 | `processEvent`, `DONE_WATCHING_VIDEO` | work%-on-target deadband |
| `diffIncrease` | 0.05 | `processEvent` and `adjustTopicDifficulty` | proportional step (× current diff) |
| `minDiff` | 3.0 | `processEvent` and `adjustTopicDifficulty` | difficulty floor in the adjusters |
| `recentPast` | 900s (15 min) | `processEvent`, `DONE_WATCHING_VIDEO` | work% lookback window |
| `MinTargetDifficulty` | 3.0 | `difficulty.go` const | floor on a user-set `target_difficulty` |
| `problemSelectionEpsilon` | 1.5 | `generate_problems.go` const | selection window half-width |
| `minAttempts` (topic) | 10 | `adjustTopicDifficulty` const | min window before a topic difficulty moves |
| weak-topic threshold | accuracy < 0.60, attempts ≥ 10 | `chooseWeightedTopic` | 2× lottery weight |
| topic harder threshold | accuracy > 0.80 | `adjustTopicDifficulty` | bump per-topic difficulty up |
| topic easier threshold | accuracy < 0.50 | `adjustTopicDifficulty` | drop per-topic difficulty down |
| `spacedRepIntervals` | `[1, 3, 7]` days | `spaced_repetition.go` var | review schedule |
| `thinPoolBoostMax` | 4.0 | `pool_supply.go` const | max thin-pool lottery multiplier |

## The global work-load adjuster

Fires once per session, on `DONE_WATCHING_VIDEO` (`processEvent`). It compares the user's recent
work percentage (work / work+watch over the last `recentPast` of events) against their
`target_work_percentage`:

- **Within `epsilon`** → on target, no change.
- **Too easy** (target work% > actual): bump `gamestate.target` by one; only once at `maxTarget`
  does it touch difficulty — halve the problem target (floored at `minProbs`) and raise
  `target_difficulty` by one step, clamped to the ceiling. At the ceiling it just resets the problem
  target and never bumps difficulty past it.
- **Too hard** (target work% < actual): halve `gamestate.target` while above `minProbs`; once at the
  floor, lower `target_difficulty` by one step (floored at `minDiff`) and bump the problem target
  back up by one.

It then runs `adjustTopicDifficulty` over every topic row, resets `gamestate.Solved`, and picks a
new reward video.

The adjuster ratchets `target_difficulty` upward on success, so it is clamped to the envelope
ceiling at two points: a standalone repair clamp at entry (`processEvent`, the
`TargetDifficulty > maxDiff` branch — persisted immediately and emitted as a `SET_TARGET_DIFFICULTY`
audit event) and the per-step `newDiff > maxDiff` guard. Without the ceiling the target drifts above
anything the envelope can produce — an empty band by construction — and the selection window never
matches again, so every serve falls through to the synchronous fallback (permanent churn). See
`MaxDiffForBitmap` in problem-generation.md.

## Per-topic stats and difficulty

`topic_stats` holds `(user_id, problem_type, attempts, correct, target_difficulty)` — one row per
**single** enabled bit (not bitmaps), only for bits in `WEIGHTED_TOPIC_MASK`.

| Function | Trigger | Effect |
|---|---|---|
| `initTopicStats` | new user / settings change (`custom_handlers.go`, `customCreateOrUpdateUser`) | `INSERT IGNORE` a row per masked enabled bit at base difficulty |
| `recordTopicAttempt` | every `ANSWERED_PROBLEM`, correct or not (`processEvent`) | for each masked bit set in the problem's bitmap, increment attempts and (if correct) correct |
| `adjustTopicDifficulty` | once per session, via the global adjuster (`processEvent`) | per topic with `attempts ≥ minAttempts`, move difficulty by accuracy, then reset attempts/correct to 0 |
| `getEffectiveDifficulty` | selection | per-topic difficulty if a row exists, else base |

**Per-topic move** (`adjustTopicDifficulty`): accuracy > 0.80 → harder by one step; accuracy < 0.50
→ easier by one step (floored at `minDiff`); between → unchanged. Counters reset after every window
regardless of whether difficulty moved, so each window is a fresh sample.

The **three accuracy thresholds are deliberately not unified**: the lottery flags a topic *weak* at
< 0.60 (extra serving weight), while the difficulty adjuster only moves at < 0.50 (easier) / > 0.80
(harder). A topic at 0.55 accuracy gets more practice at the same difficulty before its difficulty
drops.

## The serving lottery (`chooseWeightedTopic`)

Picks the single topic to focus a serve on. Two independent weight signals multiply:

- **Skill / demand** (this area): a topic with `attempts ≥ 10` and `accuracy < 0.60` gets weight 2;
  the chosen topic is served at its own per-topic difficulty.
- **Pool supply** (`pool_supply.go`, owned by problem-generation.md's Selection section): thin pools
  get up to `thinPoolBoostMax` weight, relative to the candidate average. Empty `poolCounts`
  disables it.

Only `WEIGHTED_TOPIC_MASK` bits are candidates. With no candidates it returns difficulty `base` and
topic 0, and `selectProblem` falls back to non-topic selection.

## Spaced repetition (`spaced_repetition.go`)

A wrong answer schedules a review; correct answers advance it through `spacedRepIntervals = [1, 3,
7]` days, then retire it.

| Function | Trigger | Effect |
|---|---|---|
| `addToReviewQueue` | wrong `ANSWERED_PROBLEM` (`processEvent`) | upsert into `review_queue` at interval 1, due in 24h; re-failing an in-queue problem resets it to interval 1 |
| `advanceReviewQueue` | correct `ANSWERED_PROBLEM` (`processEvent`) | if in-queue, advance to the next interval; past the last interval, delete it |
| `getDueReviewProblem` | start of `selectProblem` (`generate_problems.go`) | earliest due, settings-matched review id, else 0 |

`getDueReviewProblem` is consulted **before** the topic lottery — a due review preempts normal
selection. It gates the queued problem against *current* settings so a now-disabled topic stops
surfacing: due now, not disabled, nonzero bitmap that is a subset of the enabled bitmap, and
difficulty within `target_difficulty + problemSelectionEpsilon`. **No lower difficulty bound** — a
now-easy review is still a meaningful retest. The subset clause matches the Stage 1/2 selection SQL
(problem-generation.md, `getSatisfyingProblemIds`).

## Invariants

- **Magnitude bits never get topic stats.** `recordTopicAttempt`, `initTopicStats`, and
  `chooseWeightedTopic` all skip bits outside `WEIGHTED_TOPIC_MASK` (= all bits except the magnitude
  bits, `problem_type.go`). Magnitude IS difficulty, so "weak at LARGE_NUMBERS → serve large numbers,
  easier" fights itself; size progression is `target_difficulty`'s job. The three sites must agree,
  or a seeded magnitude row would feed `getEffectiveDifficulty` a meaningless per-topic difficulty.
- **No difficulty lever exceeds the envelope ceiling.** Both `SET_TARGET_DIFFICULTY` validation
  (`processEvent`) and the work-load adjuster clamp to `MaxDiffForBitmap`. Per-topic difficulties
  are not independently clamped here — they ride on the global ceiling through the same selection
  window.
- **No difficulty lever drops below its floor.** Adjusters floor at `minDiff = 3.0`; user-set
  targets floor at `MinTargetDifficulty = 3.0` (`difficulty.go`).
- **Topic windows reset every adjustment.** `adjustTopicDifficulty` zeroes attempts/correct after
  each window, moved or not — so accuracy is always measured over a fresh ≥`minAttempts` sample.

## Gotchas / non-obvious behavior

- **`recordTopicAttempt` uses the problem's bitmap, not the focused topic.** A multi-bit problem
  credits *every* masked bit it carries, so one answer updates several topics' stats at once.
- **`SET_TARGET_DIFFICULTY` validation accepts the bitmap ceiling but its error text shows the
  global floor.** The message bounds are `MinTargetDifficulty` and the bitmap-derived ceiling
  (`processEvent`, the `SET_TARGET_DIFFICULTY` branch) — the lower bound shown is the global floor,
  not a per-bitmap value.
- **Global vs per-topic difficulty can diverge.** A kid can have a low global `target_difficulty`
  but a high per-topic one (or vice versa); selection uses the per-topic value whenever a row exists
  (`getEffectiveDifficulty`), so the global lever only governs topics without rows and the fallback
  path.
- **The adjuster only runs on `DONE_WATCHING_VIDEO`.** Difficulty does not move mid-session; it
  re-tunes once, at the reward boundary, over the last 15 minutes of work/watch events.
- **The per-topic pass is best-effort.** A `getTopicStats` failure logs and skips it (`processEvent`,
  the `DONE_WATCHING_VIDEO` branch); the session still completes.

## Related files

- `server/api/process_events.go` — `processEvent`: event dispatch, the global work-load adjuster
  (`DONE_WATCHING_VIDEO`), `SET_TARGET_DIFFICULTY` validation, and the `recordTopicAttempt` /
  review-queue hookups on `ANSWERED_PROBLEM`.
- `server/api/topic_stats.go` — `topic_stats` CRUD, `adjustTopicDifficulty`, `chooseWeightedTopic`,
  `getEffectiveDifficulty`, `initTopicStats`.
- `server/api/spaced_repetition.go` — `addToReviewQueue`, `advanceReviewQueue`, `getDueReviewProblem`.
- `server/mathcore/difficulty.go` — `MinTargetDifficulty`; `MaxDiffForBitmap` (the ceiling) and the
  formula are owned by problem-generation.md. (The formula kernel now lives in the shared
  `server/mathcore` package; `process_events.go`/`topic_stats.go` import it.)
- `server/mathcore/problem_type.go` — `WEIGHTED_TOPIC_MASK`.
- `server/api/generate_problems.go` — `selectProblem` (caller), `problemSelectionEpsilon`.
- `server/api/pool_supply.go` — `thinPoolBoost`, `thinPoolBoostMax`.
- `web/src/bitmap_validation.js` — `MIN_TARGET_DIFFICULTY` slider mirror.
