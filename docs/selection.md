# Selection & serving

How the server chooses which already-generated problem to serve a user on each
request, keeps the candidate pool healthy, and avoids recent repeats. The
*content* of the pool (bits, difficulty, generation, validation) is owned by
`docs/problem-generation.md`; this doc owns the **picking**: the candidate SQL,
the recency bias, and the recently-shown cache.

**Update contract.** If you change selection behavior (the candidate SQL, the
selection constants, the recency/trim sizing) update this doc in the same PR.
`make docs-check BASE=origin/master` flags an untouched doc when its owned
files change; the doc-sync test (`TestDocsSyncSelection`, docs_sync_test.go)
fails CI when the anchors below disagree with the code constants.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
recency_window: 50
lru_top_frac: 0.20
selection_epsilon: 1.5
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

Within those bounds, *which* candidate is decided by a **recency bias**: the
pick is uniform among the least-recently-shown ids.

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

## The selection pipeline

`selectProblem` (generate_problems.go) runs these stages in order, taking the
first that yields a servable problem:

```
[0] SPACED-REP   getDueReviewProblem: earliest due review_queue row still
                 matching the envelope + difficulty UPPER bound + status active.
                 (spaced_repetition.go) Serve it directly if still available.
[1] DEFAULT      getSatisfyingProblemIds over the whole envelope; recency-bias
                 pick. Pool < minSelectionPool AND WORD enabled -> background WORD
                 generation (the expensive narration; non-WORD needs no batch).
[2] HEURISTIC    no servable pooled problem: generate a non-WORD problem live via
                 the in-process heuristic (envelope &^ WORD) — the immediate answer
                 AND how non-WORD problems enter the pool (they get no batch).
[2b] RELAX       the heuristic minted nothing new (every candidate collided with an
                 existing row): the difficulty band is saturated and the recency
                 exclusion had masked those rows in stage 1. Re-run getSatisfying-
                 ProblemIds with an empty prevIds and recency-bias pick — a brief
                 repeat beats failing the request.
[3] BLOCK        no non-WORD skeleton to build (a WORD-only envelope): block on a
                 synchronous WORD generateProblems call. WORD-only is invalid per
                 the settings rules and yields nothing.
```

`selectProblem` is pure with respect to the HTTP response: every stage returns a
problem or an error and **never writes to the request context**. The caller
(`processEvent` / the custom handlers) owns response writing, so a recoverable
pooled-fetch error just drops through to the next stage instead of half-writing a
response.

Stage 1 prefers newer generators: `newestVersionTier` runs the
satisfying-set query, buckets candidates by `generatorRank` (generator_rank.go),
and returns only the **highest-ranked version present**, falling back to older
versions only when no newer one matches. The ranked set is the `llm_*` WORD
narrators top (`llm_0.8` > `llm_0.7` > `llm_0.6`, newest first), then the
deterministic heuristics (`heuristic_2.2` > `heuristic_2.1` > `heuristic_2.0`);
every pre-`llm_0.6` version is retired (`status = 'deprecated'`, migration 47) and
dropped from the satisfying set before ranking ever runs. An unranked/legacy
generator string ranks 0, below every ranked version, so a stray still-`active`
legacy row is preferred last. Since the `llm_*` generators produce only WORD
problems and the
heuristic produces only non-WORD, they never compete in a cell — a WORD cell
draws the newest `llm_*` rows present, a symbolic cell the newest
`heuristic_*` rows present. Version
provenance — what each generator
string means — is owned by `docs/generator-versions.md`.

The hard-exclusion list (`prevIds`) is the `recentProblemHistorySize`
most-recently-shown ids, loaded by `loadRecentProblemIds` (process_events.go)
and threaded into the candidate SQL as an `id NOT IN (...)` clause
(`getSatisfyingProblemIds`).

## The candidate SQL

Two queries share the envelope + window clause; the covering index makes the
subset filter cheap.

| Query | Extra clause | Cite |
|---|---|---|
| `getSatisfyingProblemIds` | — (whole envelope) | generate_problems.go |
| `getDueReviewProblem` | JOIN `review_queue`; difficulty upper bound only | spaced_repetition.go |

Only `status = 'active'` rows are servable; `deprecated`/`reported`/`incorrect`
are excluded (see `docs/schema.md` for the state semantics).

Index `idx_problems_status_diff_bitmap` on `(status, difficulty,
problem_type_bitmap)` — equality seek on `status='active'`, then the difficulty
range, with the trailing bitmap column making the subset filter covering. It
replaced the byte-identical `(disabled, …)` index in `migrations/46.sql` (plans
and timing in `migrations/39.sql`).

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

- **Subset + non-zero, everywhere.** Both candidate queries carry
  `(problem_type_bitmap & ~enabled) = 0 AND problem_type_bitmap != 0`. A
  selection path that omits either is a leak.
- **Stored difficulty is per-problem, not per-request.** The pool is shared; the
  difficulty window is applied at query time, never baked into a row.
- **Background generation is WORD-only and never blocks the happy path.** Stage 1
  only *kicks off* narration on a thin pool when WORD is enabled (the LLM path
  worth pre-generating); non-WORD is generated inline by the cheap heuristic
  (stage 2). Only stages 2–3 (no servable pooled problem) generate synchronously.
- **Single-flight background generation per user.** `generateProblemsBackground`
  dedups concurrent runs per user via `backgroundGenLocks` (a `sync.Map` of
  mutexes); losers log and skip. Without it the 500ms working-on-problem ticker
  would stack goroutines over one slow LLM round-trip.
- **Cache failures degrade, never deny.** `loadRecentProblemIds` and
  `lastShownAt` fall back (empty exclusion / uniform random) rather than fail
  the request.

## Gotchas

- **`getDueReviewProblem` has no lower difficulty bound** — a now-easy review is
  intentionally still served (`getDueReviewProblem`, the difficulty-upper-bound-only
  clause). `getSatisfyingProblemIds` is two-sided.
- **Background generation requests a larger batch than the sync fallbacks.**
  `generateProblemsBackground` asks for 20 problems while the synchronous
  fallbacks request fewer — sizing differs by path. The thin-pool trigger fires
  at `minSelectionPool` (100), well above the refill batch, so a thin pool is
  refilled over several requests.

## Related files

- `server/api/generate_problems.go` — `selectProblem`, the candidate SQL,
  `newestVersionTier`, background-generation single-flight, the constants.
  (Bit detection, `DetectProblemTypeBitmap`, now lives in the shared
  `server/mathcore` kernel, not here — see [problem-generation.md](problem-generation.md).)
- `server/api/select_lru.go` — `pickWithRecencyBias`, `recencyLess`,
  `lastShownAt`.
- `server/api/trim_recently_shown.go` — `TrimRecentlyShownProblems`,
  `planRecentlyShownTrim`.
- `server/api/spaced_repetition.go` — `getDueReviewProblem`.
- `server/api/process_events.go` — `loadRecentProblemIds`, `recordRecentlyShown`
  (cache write).
- `server/api/generator_rank.go` — `generatorRank`, the rank ordering selection
  prefers (version meanings owned by `docs/generator-versions.md`).
- `server/api/migrations/39.sql` — the covering selection index.
