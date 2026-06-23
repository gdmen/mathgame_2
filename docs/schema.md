# Data model & schema

The canonical reference for the database: the core tables, the
`models.json` → generated-Go pipeline that defines most of them, and the
startup migration runner. **If you change the schema — a `models.json`
field, a new migration, a new table — update this document in the same PR.**

Mechanical enforcement: `make docs-check BASE=origin/master` fails when an
owned file (`server/api/models.json`, `server/api/migrations/**`) changes
without this doc, and the doc-sync test (`server/api/docs_sync_test.go`
`TestDocsSyncSchema`) fails CI when the anchors below disagree with the
code — the latest migration number and the model-table inventory cannot
drift undocumented.

<!-- BEGIN DOC-SYNC ANCHORS (parsed by server/api/docs_sync_test.go) -->
```
latest_migration: 43
model_tables: users, problems, playlists, videos, settings, gamestates, events
```
<!-- END DOC-SYNC ANCHORS -->

## The model

There are two sources of table definitions, and a fresh DB and a long-lived
migrated DB must **converge to the same schema**:

| Source | Defines | Applied when |
|---|---|---|
| `server/api/models.json` → `*_model.generated.go` `CreateXTableSQL` | the 7 CRUD-managed tables (`users`, `problems`, `playlists`, `videos`, `settings`, `gamestates`, `events`) | fresh DB only — `NewApi` runs `CREATE_TABLES_SQL`, ignoring "already exists" |
| `server/api/init.go` `CREATE_TABLES_SQL` | the 7 generated tables **plus** 3 hand-written join tables (`playlist_video`, `user_playlist`, `user_has_video`) | fresh DB only, same `NewApi` loop |
| `server/api/migrations/<N>.sql` | every schema **change** since the tables were first created, **plus** auxiliary tables never modelled in Go (caches, queues, stats) | every startup, in numeric order |

The generated `CreateXTableSQL` is the table's shape *as it exists today*;
migrations are the *diff history* that brings an already-deployed DB up to
that shape. A column that `models.json` shows today was added to a
deployed DB by some migration. The two MUST agree on the end state — see
the playlist worked example in Gotchas.

`NewApi` (`server/api/init.go` `NewApi`) creates missing tables on a fresh
DB; `RunMigrations` (`server/api/migrate.go` `RunMigrations`) applies the
diff history. At startup `RunMigrations` runs **before** `NewApi` — see the
Startup section for why both DBs still converge.

## The `models.json` → generated-Go pipeline

`make build-api` runs `server/code_generation/generate_models.py` (and
`generate_handlers.py`) over `server/api/models.json`, writing one
`<name>_model.generated.go` per model. **Edit the JSON and regenerate;
never hand-edit `*.generated.go`**. `make clean` deletes the
generated files; `make build-api` is a prerequisite of `make test`.

Each model row is `{name, table, fields[]}`; each field is
`{name, type, sql}` where `name` is Go/PascalCase, `type` is the Go type,
and `sql` is the column's DDL fragment. `_note` keys document intent and
are ignored by codegen.

`get_model_string` (`server/code_generation/generate_models.py`) derives,
per model, from the field list:

| Generated artifact | Driven by |
|---|---|
| `CreateXTableSQL` | `name snake_case + sql` joined per field — the full `CREATE TABLE` |
| `createXSQL` (INSERT column list) | fields with **no `DEFAULT`** and not `AUTO_INCREMENT` (`create_sql_fields`) — a `DEFAULT` column is omitted from INSERT so the DB fills it |
| struct + JSON/`uri`/`form` tags | every field; `form` tag omitted on the PRIMARY KEY field |
| `Get`/`List`/`Update`/`Delete` + `CustomList`/`CustomIdList`/`CustomSql` | the PRIMARY KEY field; a `UserId` field adds a user-scoped `WHERE` |

Codegen reads the `sql` string structurally — substring matches on
`PRIMARY KEY`, `AUTO_INCREMENT`, `DEFAULT`, `UNIQUE`, and
`TIMESTAMP`/`DATE`/`DATETIME` (the last toggles the `time` import). The
column name is `CAMEL_TO_SNAKE_RE` applied to the field name, so
`SymbolicExpression` → `symbolic_expression`, `YouTubeId` → `you_tube_id`.

## Table reference

Modelled tables (defined by `models.json`, full field list there):

| Table | Model | Key | Purpose |
|---|---|---|---|
| `users` | `user` | `auth0_id` (PK), `id` (auto, unique) | account; `role` defaults `'student'` (migration 41) |
| `problems` | `problem` | `id` | the generated problem pool; bitmap, expression, answer, difficulty, `symbolic_expression` (migration 43), `generator`, `difficulty_version` (migration 38) — see `docs/problem-generation.md` |
| `settings` | `settings` | `user_id` | per-user envelope: `problem_type_bitmap`, `target_difficulty`, `target_work_percentage` |
| `gamestates` | `gamestate` | `user_id` | current served problem/video + solved/target counters |
| `events` | `event` | `id` (auto) | append-only event log; `event_type` + `value` |
| `videos` | `video` | `id` (auto) | reward videos; `you_tube_id` `NULL UNIQUE` |
| `playlists` | `playlist` | `id` (auto) | YouTube playlists |

Hand-written join tables (`server/api/init.go`, fresh DB; re-asserted
`IF NOT EXISTS` by migrations 19/20/21 for already-deployed DBs):

| Table | Key | Purpose |
|---|---|---|
| `playlist_video` | `(playlist_id, video_id)` | playlist membership; FKs to both |
| `user_playlist` | `(user_id, playlist_id)` | which playlists a user has |
| `user_has_video` | `(user_id, video_id)` | which videos a user has |

Migration-only tables (never modelled in Go; created and owned entirely by
migrations, read by the cmd tools / serving paths named):

| Table | Migration | Read by |
|---|---|---|
| `schema_migrations` | runner (`createSchemaMigrationsTable`) | the migration runner — records applied versions |
| `statistics_cache_meta`, `statistics_totals`, `statistics_monthly`, `statistics_hardest_aggregates` | 16 | `cmd/update_statistics_cache`, statistics handler |
| `compress_events_meta` | 28 | `cmd/compress_events` |
| `topic_stats` | 29 | `topic_stats.go` per-topic difficulty (`docs/selection.md`) |
| `review_queue` | 31 | spaced-review selection (`getDueReviewProblem`) |
| `recently_shown_problems` | 36 | `process_events.go` exclude + `select_lru.go` staleness sort |
| `calibration_report` | 42 | admin difficulty-calibration cache (single row `id=1`) |

## The migration runner

`RunMigrations` (`server/api/migrate.go`):

```
[1] ensure schema_migrations exists       (CREATE TABLE IF NOT EXISTS)
[2] read applied versions                  (SELECT version FROM schema_migrations)
[3] if NEVER migrated (applied empty):     record 1..14 WITHOUT running them
[4] for each migrations/*.sql in numeric   skip if recorded; else runOne; then
    order by base name:                     INSERT its version into schema_migrations
```

- **Discovery & ordering.** Migration files are `//go:embed`-ed
  (`migrationsFS`). Names must be `<int>.sql`; a non-numeric name is logged
  and **skipped** (not an error). Versions are sorted **numerically**, so
  `2.sql` runs before `10.sql` (string sort would not).
- **The 1–14 skip.** On a DB whose `schema_migrations` is empty,
  `RunMigrations` inserts versions 1–14 as already-applied without running
  them — every environment had those applied manually before this runner
  existed. Controlled by the `skipOneThroughFourteen` arg; the exported
  `RunMigrations` passes `true`, tests can pass `false`.
- **Statement splitting.** `runOne` → `splitStatements` splits the file on
  `;` and runs each non-empty piece separately (the Go MySQL driver runs
  one statement per `Exec`). This is a *dumb* split: **no semicolons inside
  SQL comments**, and a multi-statement `PREPARE`/`EXECUTE` idempotency
  guard is written as separate `;`-terminated statements on purpose.
- **Atomicity.** Each statement is its own `Exec` — there is no surrounding
  transaction. A migration that fails partway leaves its earlier statements
  applied and is **not** recorded, so it re-runs from the top next startup.
  This is why every statement must be a no-op on re-run (below).

## Invariants

1. **Re-run safety.** A new migration statement must be a no-op when
   re-applied AND when applied to a fresh schema (whose generated
   `CREATE TABLE` may already include the column) — because a migration that
   fails partway re-runs from the top (see Atomicity). The repo's idioms:
   - `CREATE TABLE IF NOT EXISTS …` for new tables.
   - For `ALTER`/`CREATE INDEX`: an `INFORMATION_SCHEMA.COLUMNS` /
     `.TABLES` / `.STATISTICS` `COUNT(*)` guard selected into `@sql`, then
     `PREPARE`/`EXECUTE`/`DEALLOCATE` — the no-op branch is `'SELECT 1'`
     (migrations 41, 42, 43 are the current template).
   - `INSERT IGNORE` / `DELETE … JOIN` for data backfills.

   The history is not uniformly re-run-safe — migration 16 uses bare
   `CREATE TABLE` (no `IF NOT EXISTS`), safe only because it is recorded
   after a full success and so never re-runs (see Gotchas). Hold the idioms
   above for everything new.
2. **No semicolons inside comments.** The splitter is naive; a `;` in a
   `--` comment becomes a bogus statement.
3. **Generated CREATE and migration history converge.** A column added by
   a migration must also appear in `models.json` (so a fresh DB gets it),
   and a column dropped by a migration must be removed from `models.json`.
   Neither path is authoritative alone — they must describe the same end
   state.
4. **`utf8mb4` everywhere.** Every `CREATE TABLE` ends
   `DEFAULT CHARSET=utf8mb4`; the app expects `utf8mb4_unicode_ci`
   collation (README "mysql" section). New tables follow suit.
5. **`models.json` is the only edit point for modelled tables.** Changing
   a `*.generated.go` directly is undone by the next `make build-api`.

## Startup order

`cmd/apiserver/main.go`: open DB → **`RunMigrations(db)`** → **`NewApi(db, cfg)`**.
Migrations run *before* `NewApi`'s `CREATE TABLE` loop. On an existing DB
the `CREATE_TABLES_SQL` loop is all "already exists" no-ops; on a brand-new
DB the migrations that pre-date a table are recorded-or-run first, then
`NewApi` creates the tables, then later migrations `ALTER` them — so the
per-statement `INFORMATION_SCHEMA` guards (which see the fresh generated
shape) are what keep both DBs converging. `cmd/compress_events` and
`cmd/update_statistics_cache` also call `RunMigrations` on startup.

## Gotchas

- **Worked example — playlists converge.** A DB first migrated at v18 got
  `playlists` with a `curated` column and no `title` (migration 18);
  migration 22 drops `curated`, migration 27 adds `title`. The end state
  matches today's generated `CreatePlaylistTableSQL` (`title`, no
  `curated`). A fresh DB gets the final shape directly from `init.go`;
  migrations 18/22/27 are no-ops on it (the `IF NOT EXISTS` / column-count
  guards). This is invariant 3 in action — do the same for any
  add-then-remove column.
- **Migration 16 is not idempotent.** Its four `statistics_*` tables use
  bare `CREATE TABLE`, not `CREATE TABLE IF NOT EXISTS`. It survives only
  because the runner records it after a full success and never re-runs it —
  but if it ever failed *after* the first `CREATE`, the retry would error on
  the already-created table and wedge startup. Don't copy this shape; new
  table-creation migrations use `IF NOT EXISTS` (migration 28+).
- **`thumbnailurl`, not `thumbnail_url`.** The field is named `ThumbnailURL`
  but `URL` is a single capitalized run, so `CAMEL_TO_SNAKE_RE` (which only
  splits before `[A-Z][a-z]`) leaves it as one token → column
  `thumbnailurl`. Adjacent-capital field names do not snake-split.
- **`DEFAULT` removes a column from INSERT.** `models.json` deliberately
  omits `DEFAULT` on `problem.SymbolicExpression` so codegen includes it in
  `createProblemSQL` (generator paths always set it); migration 43 adds the
  column with `DEFAULT ''` purely to backfill existing rows. The same
  pattern governs `problem.DifficultyVersion` (migration 38). The `_note`
  on each field records the reasoning. Adding a `DEFAULT` to a `models.json`
  field silently drops it from the INSERT column list.
- **`id` types are uneven.** Most models declare `Id uint32` over a
  `BIGINT UNSIGNED` column. Don't assume the Go width matches the SQL width.
- **Know the true current shape before destructive prod SQL.** Because the
  schema is a CREATE plus a diff history, a column's defining migration is not
  the whole story — a later migration may have dropped or renamed it (the
  playlists example above). Before any irreversible prod statement, grep the
  **full** `migrations/**` history for `DROP`/`RENAME`/`CHANGE` on the table,
  not just its `CREATE`. A half-check is worse than none.
- **No down-migrations.** The runner is forward-only. To undo, write a new
  higher-numbered migration.
- **`verify_migrations`** (`cmd/verify_migrations`) is a one-off
  before/after consistency checker for the video de-dup migration (25); it
  does **not** run migrations.

## Related files

- `server/api/models.json` — the model source of truth.
- `server/code_generation/generate_models.py` `get_model_string` — codegen.
- `server/api/*_model.generated.go` — generated tables/CRUD (do not edit).
- `server/api/init.go` `NewApi`, `CREATE_TABLES_SQL` — fresh-DB table creation + join tables.
- `server/api/migrate.go` `RunMigrations`, `splitStatements` — the runner.
- `server/api/migrations/<N>.sql` — the diff history (latest: 43).
- `server/api/docs_sync_test.go` `TestDocsSyncSchema` — anchor enforcement.
- README "mysql" section — charset/collation + DB-creation runbook.

## Adding to the schema — checklist

Modelled-table change (new column on an existing managed table):
1. Edit the field's row in `server/api/models.json` (Go name, type, `sql`).
2. `make build-api`; never touch `*.generated.go` by hand.
3. Add `server/api/migrations/<N+1>.sql` with an `INFORMATION_SCHEMA`-guarded
   `ALTER` so deployed DBs catch up and re-runs/fresh DBs are no-ops.
4. Bump `latest_migration` in this doc's anchor block to `N+1`.
5. If the column needs backfilling on deploy, note the cmd tool / run order
   (see `docs/problem-generation.md` deploy section for the difficulty
   example).

New auxiliary (un-modelled) table:
1. Add `server/api/migrations/<N+1>.sql` with `CREATE TABLE IF NOT EXISTS`.
2. Add a row to the migration-only table reference above with its reader.
3. Bump `latest_migration` to `N+1`.

New managed model (rare):
1. Add the `{name, table, fields}` block to `models.json`; `make build-api`.
2. Wire its `CreateXTableSQL` into `CREATE_TABLES_SQL` (`server/api/init.go`)
   and a manager into `NewApi`.
3. Add the table to `model_tables` in the anchor block + the Table reference.
