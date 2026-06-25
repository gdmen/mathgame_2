# Events: compression and the statistics cache

Source of truth for keeping the append-only `events` table compact (run-length compression of
duration events) and rolling it up into the per-user statistics cache behind the progress page.
**Change this doc in the same PR as any behavior change here.** `docs_sync_test` (`TestDocsSyncEvents`)
pins the anchor block below to the code and fails CI on drift, so an event-type rename or a new
summable / counted type cannot land undocumented.

This area owns `event_types.go` (the event-type vocabulary), `event_compress.go`,
`statistics_handlers.go`, and their two job commands
(`cmd/compress_events`, `cmd/update_statistics_cache`). The `ProblemType` bits, difficulty,
and selection are a separate area (`docs/problem-generation.md`); its math kernel lives in
`server/mathcore`.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
event_types: logged_in, selected_problem, working_on_problem, answered_problem, solved_problem, error_playing_video, watching_video, done_watching_video, set_target_difficulty, set_target_work_percentage, set_problem_type_bitmap, set_gamestate_target, bad_problem_system, bad_problem_user
summable_event_types: working_on_problem, watching_video
stats_counted_event_types: solved_problem, working_on_problem, watching_video
compress_max_chunk_size: 21845
```
<!-- END DOC-SYNC ANCHORS -->

## The model

`events` is an append-only log keyed by an autoincrement `id`. Each row is a
`(id, timestamp, user_id, event_type, value)` tuple (`Event`); `event_type` is one of the string
constants in `event_types.go`, and `value` is a type-specific opaque string (each constant's comment names
what it carries — duration ms, a problem id, an answer, etc.).

Two derived structures sit alongside the log, each advanced by its own checkpoint so neither rescans
history:

| Structure | Tables | Checkpoint | Built by |
|---|---|---|---|
| **Compressed log** | `events` (in place) | `compress_events_meta.last_event_id` (single global row) | `event_compress.go` |
| **Statistics cache** | `statistics_totals`, `statistics_monthly` | `statistics_cache_meta.last_event_id` (per user) | `statistics_handlers.go` |

The checkpoints are independent. Compression rewrites `value` and deletes rows; the statistics cache
only ever reads. Schemas: `compress_events_meta` in `migrations/28.sql`; the three statistics tables
in `migrations/16.sql`.

## Event-type roles

Each event type plays at most one role in this area, defined by three maps (the anchors pin the
membership of the first two):

| Role | Where | Members | Meaning |
|---|---|---|---|
| **Summable** | `summableEventTypes` | `working_on_problem`, `watching_video` | `value` is a duration (ms); consecutive same-user runs may collapse to one summed row. |
| **Counted by stats** | the `event_type IN (...)` lists in `fullProgressBackfill` / `mergeProgressEventsIntoCache` | `solved_problem` (counted), `working_on_problem` (work ms), `watching_video` (video ms) | The only types the statistics cache reads. |
| **Record-only** | `recordOnlyEventTypes` | `logged_in`, `working_on_problem`, `watching_video`, `set_target_work_percentage` | Persisted but don't mutate gamestate/settings — **owned by the event-processing area, not this doc**; listed only to contrast. |

A type may hold more than one role: `working_on_problem` and `watching_video` are both summable and
counted; `solved_problem` is counted but not summable (its `value` is a problem id, not a duration).
Every other type passes through both jobs untouched.

## Compression

`CompressEvents` is the pure core: it collapses each maximal run of consecutive same-`(user_id,
event_type)` summable rows into one row carrying the summed duration. The first row of a run is
emitted as an update with `value` = sum; the rest are emitted as deletes. `RunCompress` is the
transactional driver and `PlanCompress` its read-only dry-run twin.

```
[scan]    SELECT ... WHERE id > last_event_id ORDER BY user_id, id   (selectEventsAfterIDSQL)
[plan]    CompressEvents -> updates (summed first rows), toDelete (rest of each run)
[apply]   chunked UPDATE ; chunked DELETE ; advance checkpoint to max scanned id
          -- all in ONE transaction; rollback on any error
```

Duration parsing is tolerant (`parseEventDurationMs`): integer first, then float truncated toward
zero, because `watching_video` posts float strings. A value that cannot be parsed at all is not an
error — it **breaks the run** so it can't corrupt the sum.

### Invariants

- **Total preserved.** Compression only reduces row count; the summed duration a user accumulated
  never changes. `TestRunCompress_Integration_ProgressUnchanged` asserts the statistics cache is
  byte-identical before and after a compress run — the two derived structures must agree.
- **Input must be sorted by `(user_id, id)`.** Run detection only looks at adjacency, so the scan's
  ordering (`selectEventsAfterIDSQL`) is load-bearing; out-of-order input would mis-group runs.
- **Idempotent via checkpoint.** A run only ever scans `id > last_event_id`, so already-compressed
  rows are never revisited. The checkpoint advances to the max scanned id even when nothing
  compressed.
- **Chunked under MySQL's placeholder ceiling.** `maxChunkSize` (anchored) caps rows per statement;
  the value is derived from the UPDATE's three placeholders per row against MySQL's 65,535-placeholder
  limit. The DELETE uses one per row and reuses the same cap.

### Gotchas

- A non-parseable `value` does not abort — it ends the current run and is left as-is, silently
  splitting what would have been one compressed row into two.
- The compress checkpoint is a single global row, not per-user, even though runs are per-user. That
  is correct because the scan is ordered `(user_id, id)` and the watermark is on `id`: a later run
  picks up every event appended after the last scanned id regardless of user.

## Statistics cache

`UpdateStatisticsForUser` maintains a per-user rollup of three numbers — total problems solved, total
work minutes, total video minutes — at all-time and per-month (`YYYY-MM`) granularity. It is the
read-side of `events` for the progress page, served by `GET /api/v1/statistics/:user_id`
(`getStatistics`), which refreshes the cache for the requesting user and reads it back. A user may
only request their own stats (403 otherwise — the `params.UserId != user.Id` check in `getStatistics`).

Two write paths, chosen on whether a per-user checkpoint row exists in `statistics_cache_meta`:

| Path | When | Write semantics |
|---|---|---|
| `fullProgressBackfill` | no meta row yet | one aggregate `SELECT`, then upsert that REPLACES the cached totals |
| `mergeProgressEventsIntoCache` | meta exists | scan only `id > last_event_id`, accumulate deltas in Go, then upsert that ADDS to the cached totals |

Both advance `statistics_cache_meta.last_event_id` to the max scanned event id.

### The ms→minutes contract (must match across both paths)

Work and video minutes derive from summed millisecond durations, and the conversion **accumulates in
milliseconds and divides by 60000 exactly once, at the end** — never per-event, which would
round-truncate each event and lose minutes. The backfill does this in SQL; the incremental path sums
in Go and divides once, per-total and per-month. The two must stay in lockstep —
`TestUpdateStatisticsForUser_IncrementalMonthlySumsThenDivides` is the regression guard and
`TestStatistics_MsToMinutes_RoundsDown` owns the rounding behavior.

### Invariants

- **Counted types only.** Both paths read exactly `solved_problem`, `working_on_problem`,
  `watching_video`; every other type is invisible to stats. This is the `stats_counted_event_types`
  anchor.
- **Backfill ≡ replay of increments.** A full backfill and an event-by-event incremental merge must
  produce identical cache rows — hence both use the sum-then-divide rule and the same counted set.
  `TestUpdateStatisticsForUser_BackfillsCacheAndMeta` and `TestStatistics_ReturnsTotals` pin the
  totals.
- **Non-positive / unparseable durations contribute 0.** Both paths clamp before converting — the
  backfill floors each summed duration at ≥ 0 in SQL so a negative stored value can't reduce a user's
  minutes (`fullProgressBackfill`); the incremental path drops values not `> 0` (`mergeProgressEventsIntoCache`).
- **Empty user reads as zeros.** `readStatisticsFromCache` returns zeroed totals and an empty month
  list when no cache row exists (`TestStatistics_EmptyUser_ReturnsZeros`).

### Gotchas

- The incremental write is additive, so it is **NOT** idempotent if replayed over an already-merged
  window — correctness depends entirely on the `last_event_id` checkpoint never double-counting a
  range. Compression deletes event rows but cannot lower an event's `id`, so a compress run never
  invalidates the stats checkpoint.
- The handler refreshes synchronously on every GET. There is no background scheduler in this area; the
  cache only advances when something calls `UpdateStatisticsForUser` (the handler or the
  `update_statistics_cache` job).

## Job commands

| Command | Flags | What it does |
|---|---|---|
| `cmd/compress_events` | `-config`, `-dry-run` | Runs migrations, then `RunCompress` (or `PlanCompress` under `-dry-run`, which prints the plan without writing). |
| `cmd/update_statistics_cache` | `-config`, `-user_id` | Runs migrations, then `UpdateStatisticsForUser` for one user (`-user_id > 0`) or every distinct user in `events` (the default). Exits non-zero if any user failed. |

Neither is wired into a scheduler in this repo; both are operator-run. `compress_events` is safe to
re-run (checkpointed, single transaction). `update_statistics_cache` for all users is a refresh, not
a rebuild — to rebuild from scratch, truncate the `statistics_*` tables first so the backfill path
runs.

## Related files

- `server/api/event_compress.go` — `CompressEvents`, `parseEventDurationMs`, `RunCompress`, `PlanCompress`, `maxChunkSize`, `summableEventTypes`.
- `server/api/statistics_handlers.go` — `UpdateStatisticsForUser`, `getStatistics`, `fullProgressBackfill`, `mergeProgressEventsIntoCache`, `readStatisticsFromCache`.
- `server/api/event_types.go` — event-type constants, `recordOnlyEventTypes`.
- `server/api/event_model.generated.go` — the `Event` struct (generated from `models.json`; never hand-edit).
- `server/api/migrations/16.sql` — `statistics_cache_meta`, `statistics_totals`, `statistics_monthly`; `migrations/28.sql` — `compress_events_meta`.
- `server/api/event_compress_test.go`, `server/api/statistics_test.go` — own the concrete values cited above.
- `cmd/compress_events/main.go`, `cmd/update_statistics_cache/main.go` — the jobs.

## Extension checklist (adding / changing an event type's role)

1. Add or rename the constant in `event_types.go`, and update the `event_types` anchor to match the enum.
2. **Summable?** (its `value` is a duration to sum across consecutive runs) → add to
   `summableEventTypes` and to the `summable_event_types` anchor.
3. **Counted by stats?** → add it to the `event_type IN (...)` lists in BOTH `fullProgressBackfill`
   and `mergeProgressEventsIntoCache`, to their per-type accumulation switches, and to the
   `stats_counted_event_types` anchor. Confirm the sum-then-divide rule still holds for a duration.
4. Update this document and the anchors — CI fails on anchor drift.
