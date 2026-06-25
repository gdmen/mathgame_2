# Selection & serving

How the server chooses which already-generated problem to serve a user on each
request, keeps the candidate pool healthy, and avoids recent repeats. The
*content* of the pool (bits, difficulty, generation, validation) is owned by
`docs/problem-generation.md`; this doc owns the **picking**: the candidate SQL,
the two-signal topic lottery, the recency bias, and the recently-shown cache.

**Update contract.** If you change selection behavior (the candidate SQL, the
selection constants, the lottery weights, the recency/trim sizing) update this
doc in the same PR. `make docs-check BASE=origin/master` flags an untouched doc
when its owned files change; the doc-sync test (`TestDocsSyncSelection`,
docs_sync_test.go) fails CI when the anchors below disagree with the code
constants.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
recency_window: 50
lru_top_frac: 0.20
selection_epsilon: 1.5
thin_pool_boost_max: 4.0
weighted_topic_mask: addition, subtraction, multiplication, division, fractions, negatives, word, chained_operations, missing_number, mismatched_denominators, decimals, pemdas, single_variable, percentages
```
<!-- END DOC-SYNC ANCHORS -->

## The model

Selection answers one question per request: *given this user's settings, which
stored problem id do we serve?* It never generates content inline on the happy
path — it picks from the shared pool and, when the pool runs thin, kicks off
**background** generation (see `docs/problem-generation.md` for the generators
themselves). Two filters bound every candidate:

- **Envelope** — bitwise subset: a problem is eligible iff every bit it carries
  is enabled for the user, `(problem_type_bitmap & ~enabled) = 0`
  (`getSatisfyingProblemIds`). Zero-bitmap rows are excluded defensively
  (`problem_type_bitmap != 0`) — a zero bitmap is a subset of everything and
  would leak to every user.
- **Difficulty window** — `target_difficulty ± problemSelectionEpsilon`
  (`getSatisfyingProblemIds`). The spaced-rep path uses only the upper bound (an
  easy retest is still meaningful — `getDueReviewProblem`, spaced_repetition.go).

Within those bounds, *which* candidate is two lotteries deep: a **topic
lottery** picks what skill to focus on, then a **recency bias** picks the
least-recently-shown id for that topic.

## Selection constants

All defined in generate_problems.go unless noted. `recencyWindow` (50) is the
base unit; the others derive from it, so the doc states the multiple, not a
frozen number — exact values are pinned by the anchor block above and
`TestDocsSyncSelection`.

| Constant | Value | Meaning |
|---|---|---|
| `recencyWindow` | 50 | base unit for all recency sizing |
| `recentProblemHistorySize` | `recencyWindow` | most-recent ids per user, hard-excluded |
| `minSelectionPool` | `2*recencyWindow` | pool below this triggers background generation |
| `recentlyShownProblemsTrimSize` | `4*recencyWindow` | max rows/user kept in `recently_shown_problems` |
| `lruTopFrac` | 0.20 | fraction of recency-sorted pool picked from uniformly |
| `problemSelectionEpsilon` | 1.5 | additive difficulty half-window |
| `poolCountsCacheTTL` | 5 min | staleness bound on per-bit pool counts (pool_supply.go) |
| `thinPoolBoostMax` | 4.0 | cap on the thin-pool rarity multiplier (pool_supply.go) |

The weak-topic threshold (≥10 attempts, accuracy <0.60 → 2× weight) lives in
`chooseWeightedTopic` (topic_stats.go) — see `docs/problem-generation.md`'s
topic-stats discussion for the demand-side detail.

## The selection pipeline

`selectProblem` (generate_problems.go) runs these stages in order, taking the
first that yields a servable problem:

```
[0] SPACED-REP   getDueReviewProblem: earliest due review_queue row still
                 matching the envelope + difficulty UPPER bound + not disabled.
                 (spaced_repetition.go) Serve it directly if still available.
[1] TOPIC-WEIGHTED  if the user has topic_stats: chooseWeightedTopic picks a
                 focus topic + its per-topic difficulty; query the pool for
                 that topic (getSatisfyingProblemIdsForTopic); recency-bias
                 pick. Thin topic pool -> background generation.
[2] DEFAULT      getSatisfyingProblemIds over the whole envelope; recency-bias
                 pick. Pool < minSelectionPool -> background generation.
[3] HEURISTIC    pool empty: synchronously run the heuristic generator over the
                 non-WORD bits (envelope &^ WORD) so the user sees something now.
[4] LLM BLOCK    WORD-only envelope (or heuristic produced nothing): block on a
                 synchronous LLM generate call.
```

Stages 1 and 2 prefer newer generators: `newestVersionTier` runs the
satisfying-set query, buckets candidates by `generatorRank` (generator_rank.go),
and returns only the **highest-ranked version present**, falling back to older
versions only when no newer one matches. An unranked/legacy generator string
ranks 0, below every known version. The rank ordering is a selection-preference
policy (newest-first); version provenance — what each generator string means — is
owned by `docs/generator-versions.md`.

The hard-exclusion list (`prevIds`) is the `recentProblemHistorySize`
most-recently-shown ids, loaded by `loadRecentProblemIds` (process_events.go)
and threaded into the candidate SQL as an `id NOT IN (...)` clause
(`getSatisfyingProblemIds`).

## The candidate SQL

Three queries share the envelope + window clause; the covering index makes the
subset filter cheap.

| Query | Extra clause | Cite |
|---|---|---|
| `getSatisfyingProblemIds` | — (whole envelope) | generate_problems.go |
| `getSatisfyingProblemIdsForTopic` | also requires the target topic bit set | generate_problems.go |
| `getDueReviewProblem` | JOIN `review_queue`; difficulty upper bound only | spaced_repetition.go |

Index `idx_problems_disabled_diff_bitmap` on `(disabled, difficulty,
problem_type_bitmap)` — the trailing bitmap column makes the subset filter
covering (plans and timing in `migrations/39.sql`).

## The topic lottery (`chooseWeightedTopic`)

Two independent weight signals **multiply** per candidate bit
(`chooseWeightedTopic`, topic_stats.go):

- **Demand (per-kid skill)** — base weight 1.0; a *weak* topic (≥10 attempts,
  accuracy <0.60) gets 2×. The chosen topic is served at its own per-topic
  `TargetDifficulty`, not the global one.
- **Supply (pool depth)** — `thinPoolBoost` multiplies in up to `thinPoolBoostMax`
  (4×) for topics whose pool is thin relative to the average among *these*
  candidates (`thinPoolBoost`, pool_supply.go). `poolCountsByBit` caches per-bit
  `disabled=0` counts for `poolCountsCacheTTL`; on query error it returns nil,
  which **disables supply weighting for that round** rather than failing the
  selection (`poolCountsByBit`, pool_supply.go).

The final pick is an integer lottery over each candidate's weight scaled by 100
(`chooseWeightedTopic`). Only **`WEIGHTED_TOPIC_MASK`** bits are candidates:
magnitude bits (MEDIUM/LARGE_NUMBERS) are excluded because magnitude *is*
difficulty — "weak at LARGE_NUMBERS → serve large numbers, easier" fights
itself; size progression is `target_difficulty`'s job (`WEIGHTED_TOPIC_MASK`,
enums.go). The same mask gates `recordTopicAttempt` and `initTopicStats`.

## The recency bias (`pickWithRecencyBias`)

Given the candidate ids for the chosen path (`pickWithRecencyBias`,
select_lru.go):

1. Look up each id's latest `shown_at` for this user from
   `recently_shown_problems` (`lastShownAt`, select_lru.go).
2. Stable-sort by `recencyLess`: **never-shown first**, then oldest-shown →
   most-recent (`recencyLess`, select_lru.go).
3. Pick uniformly at random from the top `lruTopFrac` (0.20) of that order, at
   least 1 (`pickWithRecencyBias`).

Ids absent from the cache are treated as never-shown — which is correct, because
the cache is trimmed (below), so anything evicted is functionally forgotten and
*should* re-enter rotation.

## The recently-shown cache

`recently_shown_problems` is a derived cache (PK `(user_id, problem_id)`); the
events table is the source of truth. It feeds both the hard-exclusion list and
the recency sort.

- **Write** — after `SELECTED_PROBLEM` events land, `recordRecentlyShown` upserts
  `(user_id, problem_id, NOW())`; re-shows update `shown_at` in place rather than
  piling up (`recordRecentlyShown`, process_events.go). Failures are logged,
  never propagated — small drift self-corrects on the next select.
- **Trim** — `TrimRecentlyShownProblems` (trim_recently_shown.go) caps each user
  at `recentlyShownProblemsTrimSize` (200) rows. Two-step: a planner SELECT
  computes each user's cutoff = the `shown_at` of their `(trimSize+1)`-th
  most-recent row (MySQL session-variable ranking, 5.7-safe, no window functions
  — `planRecentlyShownTrim`), then a per-user `DELETE ... WHERE shown_at <
  cutoff`. Users with ≤ cap rows have no `(cap+1)`-th row and are skipped.
  `dryRun` returns the plan without writing; a single user's delete failure is
  logged and skipped, not fatal.

## Invariants

- **Subset + non-zero, everywhere.** All three candidate queries carry
  `(problem_type_bitmap & ~enabled) = 0 AND problem_type_bitmap != 0`. A
  selection path that omits either is a leak.
- **Stored difficulty is per-problem, not per-request.** The pool is shared; the
  difficulty window is applied at query time, never baked into a row.
- **Background generation never blocks the happy path.** Stages 1–2 only *kick
  off* generation on a thin pool; only stages 3–4 (empty pool) generate inline,
  and stage 3 prefers the synchronous heuristic over an LLM round-trip.
- **Single-flight background generation per user.** `generateProblemsBackground`
  dedups concurrent runs per user via `backgroundGenLocks` (a `sync.Map` of
  mutexes); losers log and skip. Without it the 500ms working-on-problem ticker
  would stack goroutines over one slow LLM round-trip.
- **Cache failures degrade, never deny.** `loadRecentProblemIds`, `lastShownAt`,
  and `poolCountsByBit` all fall back (empty exclusion / uniform random / no
  supply weighting) rather than fail the request.

## Gotchas

- **`getDueReviewProblem` has no lower difficulty bound** — a now-easy review is
  intentionally still served (`getDueReviewProblem`, the difficulty-upper-bound-only
  clause). The other two queries are two-sided.
- **Background generation requests a larger batch than the sync fallbacks.**
  `generateProblemsBackground` asks for 20 problems while the synchronous
  fallbacks request fewer — sizing differs by path. The thin-pool trigger fires
  at `minSelectionPool` (100), well above the refill batch, so a thin pool is
  refilled over several requests.
- **Supply weighting is relative to the candidate set, not global.** The average
  is the mean pool count among the bits enabled *for this user this round*
  (`chooseWeightedTopic`), so the same topic can be "thin" for one kid and
  average for another.
- **Topic-weighted path keeps the full envelope, then filters.** It overrides
  only `TargetDifficulty` on a settings copy and relies on the target-topic SQL
  clause to narrow — the user's whole bitmap still bounds the subset check
  (`selectProblem`).
- **Integer lottery truncates sub-0.01 weights.** Weights are scaled by 100 and
  truncated to int (`chooseWeightedTopic`); a combined weight below 0.01 rounds
  to zero. Not reachable today (min weight is 1.0×1.0), but a future sub-unit
  demand signal would silently drop candidates.

## Related files

- `server/api/generate_problems.go` — `selectProblem`, the candidate SQL,
  `newestVersionTier`, background-generation single-flight, the constants.
- `server/api/select_lru.go` — `pickWithRecencyBias`, `recencyLess`,
  `lastShownAt`.
- `server/api/pool_supply.go` — `poolCountsByBit`, `thinPoolBoost`, supply-side
  cache.
- `server/api/trim_recently_shown.go` — `TrimRecentlyShownProblems`,
  `planRecentlyShownTrim`.
- `server/api/topic_stats.go` — `chooseWeightedTopic` and the demand signal
  (detail in `docs/problem-generation.md`).
- `server/api/spaced_repetition.go` — `getDueReviewProblem`.
- `server/api/process_events.go` — `loadRecentProblemIds`, `recordRecentlyShown`
  (cache write).
- `server/api/generator_rank.go` — `generatorRank`, the rank ordering selection
  prefers (version meanings owned by `docs/generator-versions.md`).
- `server/api/enums.go` — `WEIGHTED_TOPIC_MASK`.
- `server/api/migrations/39.sql` — the covering selection index.
