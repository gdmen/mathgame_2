# Adaptive difficulty & progression

How the system moves a kid through the difficulty band: the global work-load adjuster and the
spaced-repetition review queue. The *content* of the difficulty band (the formula, the bits, the
ceiling) is owned by **`docs/problem-generation.md`**; this doc owns the *levers that move within
it*.

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

Two persisted levers move a kid through the band, bounded by the envelope band
`TargetDifficultyRange` — floor `max(MinTargetDifficulty, MinDiffForBitmap)`, ceiling
`MaxDiffForBitmap` (owned by problem-generation.md):

| Lever | Stored in | Adjusted by | Bounds |
|---|---|---|---|
| `settings.target_difficulty` | `settings` | global work-load adjuster | `TargetDifficultyRange(bitmap)` |
| `gamestate.target` (problems per session) | `gamestates` | global work-load adjuster | `[minProbs, maxTarget]` |

The difficulty lever feeds **selection** (docs/selection.md): selection draws within
`± problemSelectionEpsilon` of the global target.

The adjuster moves on **work percentage** (time-on-task). The step rule: at least a full point,
otherwise `diffIncrease` (5%) of the current difficulty — so low difficulties move by whole points
and high ones move proportionally.

## Reference table — adjustment levers

All defined as locals/consts at their use site; cite the enclosing symbol.

| Lever | Value | Where | Meaning |
|---|---|---|---|
| `maxTarget` | 20 | `process_events.go` const | ceiling on problems-per-session |
| `minProbs` | 5 | `processEvent`, `DONE_WATCHING_VIDEO` | floor on problems-per-session |
| `epsilon` | 0.05 | `processEvent`, `DONE_WATCHING_VIDEO` | work%-on-target deadband |
| `diffIncrease` | 0.05 | `processEvent` | proportional step (× current diff) |
| `minDiff`/`maxDiff` | per-bitmap | `processEvent` (from `TargetDifficultyRange`) | the adjuster's difficulty band |
| `recentPast` | 900s (15 min) | `processEvent`, `DONE_WATCHING_VIDEO` | work% lookback window |
| `MinTargetDifficulty` | 3.0 | `difficulty.go` const | global floor inside `TargetDifficultyRange` |
| `problemSelectionEpsilon` | 1.5 | `generate_problems.go` const | selection window half-width |
| `spacedRepIntervals` | `[1, 3, 7]` days | `spaced_repetition.go` var | review schedule |

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

It then resets `gamestate.Solved` and picks a new reward video.

The adjuster moves `target_difficulty` in both directions, so it is clamped to the envelope band
(`TargetDifficultyRange`) at two points: a standalone repair clamp at entry (`processEvent`, the
`ClampTargetDifficulty` branch — catches both a runaway high value and a below-floor value from
before the floor existed; persisted immediately and emitted as a `SET_TARGET_DIFFICULTY` audit
event) and the per-step guards (`newDiff > maxDiff` cap; step-down floored at `minDiff`, the band
floor). Outside the band the target sits where the envelope can produce nothing — an empty band by
construction, in either direction (a division-only envelope builds nothing easier than ~7) — and
the selection window never matches, so every serve falls through to the synchronous fallback
(permanent churn). See `TargetDifficultyRange` in problem-generation.md.

## Spaced repetition (`spaced_repetition.go`)

A wrong answer schedules a review; correct answers advance it through `spacedRepIntervals = [1, 3,
7]` days, then retire it.

| Function | Trigger | Effect |
|---|---|---|
| `addToReviewQueue` | wrong `ANSWERED_PROBLEM` (`processEvent`) | upsert into `review_queue` at interval 1, due in 24h; re-failing an in-queue problem resets it to interval 1 |
| `advanceReviewQueue` | correct `ANSWERED_PROBLEM` (`processEvent`) | if in-queue, advance to the next interval; past the last interval, delete it |
| `getDueReviewProblem` | start of `selectProblem` (`generate_problems.go`) | earliest due, settings-matched review id, else 0 |

`getDueReviewProblem` is consulted at the start of `selectProblem` — a due review preempts normal
selection. It gates the queued problem against *current* settings so a now-disabled topic stops
surfacing: due now, `status = 'active'`, nonzero bitmap that is a subset of the enabled bitmap, and
difficulty within `target_difficulty + problemSelectionEpsilon`. **No lower difficulty bound** — a
now-easy review is still a meaningful retest. The subset clause matches the default selection SQL
(docs/selection.md, `getSatisfyingProblemIds`).

## Reported problems (`bad_problem_system` / `bad_problem_user`)

Both bad-problem events (`processEvent`) mark the offending problem
`status = 'reported'` — a KaTeX render failure (`bad_problem_system`) and an
adult report (`bad_problem_user`) are both *unvalidated* claims, so they land in
the same state and neither serves nor is confirmed wrong. The write is a targeted
`UPDATE problems SET status = ? WHERE id = ?` rather than the manager's full-row
`Update`, so it can't clobber a concurrent field write. The problem id comes from
the event value (`parseBadProblemID`), falling back to `gamestate.ProblemId`; a
new problem is re-selected only when the reported one is the current problem. Only
a later admin review (a follow-up) promotes `reported` to `incorrect` or back to
`active`. State semantics are owned by `docs/schema.md`.

## Invariants

- **No difficulty lever leaves the envelope band.** `SET_TARGET_DIFFICULTY` validation
  (`processEvent`), the work-load adjuster, and the settings-save clamp all bound through
  `TargetDifficultyRange` — ceiling `MaxDiffForBitmap`, floor
  `max(MinTargetDifficulty, MinDiffForBitmap)`.

## Gotchas / non-obvious behavior

- **The adjuster only runs on `DONE_WATCHING_VIDEO`.** Difficulty does not move mid-session; it
  re-tunes once, at the reward boundary, over the last 15 minutes of work/watch events.

## Related files

- `server/api/process_events.go` — `processEvent`: event dispatch, the global work-load adjuster
  (`DONE_WATCHING_VIDEO`), `SET_TARGET_DIFFICULTY` validation, and the review-queue hookups on
  `ANSWERED_PROBLEM`.
- `server/api/spaced_repetition.go` — `addToReviewQueue`, `advanceReviewQueue`, `getDueReviewProblem`.
- `server/mathcore/difficulty.go` — `MinTargetDifficulty`; `MaxDiffForBitmap` (the ceiling) and the
  formula are owned by problem-generation.md. (The formula kernel now lives in the shared
  `server/mathcore` package; `process_events.go` imports it.)
- `server/api/generate_problems.go` — `selectProblem` (called without a `gin.Context`:
  selection returns a problem or an error and never writes the response; `processEvent`
  writes it), `problemSelectionEpsilon`.
- `web/src/bitmap_validation.js` — `MIN_TARGET_DIFFICULTY` slider mirror.
