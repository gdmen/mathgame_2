# Deploy & ops runbook

How mathgame is built, deployed, and operated: the `make` targets, the
`cmd/*` tools, the systemd units, and the deploy scripts. **If you change the
build, the deploy sequence, a `cmd/*` tool's contract, or a systemd unit,
update this doc in the same PR.** Mechanical enforcement: `make docs-check
BASE=origin/master` (`scripts/docs_check.py`, driven by the Project Areas
registry in `README.md`) fails when this area's owned files
(`deploy/**`, `Makefile`, `cmd/**`) change without this doc being touched.

This is a prose area — no DOC-SYNC anchor block. Tool-specific behavior that
the problem-generation system owns (the difficulty version bump, the bitmap
restamp invariants, the backfill ordering rationale) is documented in
`docs/problem-generation.md`; this doc covers *when and how* to run those tools
on deploy, not what they compute.

## The topology

Single server, single MySQL DB. Everything is a systemd
unit on one Ubuntu 22.04 host running under `/home/ubuntu/mathgame_2`. There is
no container, no orchestrator, no blue/green — a deploy rebuilds in place and
restarts the services. Go 1.24.2 (see First-time provisioning below).

Two long-running servers plus a stand-in:

| Service | `ExecStart` | Role |
|---|---|---|
| `mathgame-api` | `make prod-api` (`apiserver`, `GIN_MODE=release`) | the API; runs migrations on startup |
| `mathgame-web` | `make prod-web` (`serve -s build` on :443, TLS) | the static React bundle |
| `mathgame-maintenance` | `make prod-maintenance` (`maintenance_server`) | "be right back" page during the disruptive deploy window |

`mathgame-maintenance` **conflicts with** `mathgame-web`: both bind :443, and
the unit declares `Conflicts=mathgame-web.service` + `After=mathgame-web.service`
so systemd runs the stop before the start in both swap directions and neither
server fights for the port (`deploy/mathgame-maintenance.service`). All three
restart on failure (`Restart=always`, `RestartSec=1s`, burst-limited to 5 in
500s).

Five scheduled `oneshot` jobs, each a `bin/*` tool fired by a `.timer`:

| Timer | Schedule (`OnCalendar`) | Tool | Does |
|---|---|---|---|
| `mathgame-compress-events` | daily 03:00 | `compress_events` | collapses event rows (`api.PlanCompress`) |
| `mathgame-check-disabled-videos` | daily 03:30 | `check_disabled_videos --enable` | re-enables videos that became playable again |
| `mathgame-update-statistics` | daily 04:00 | `update_statistics_cache` | rebuilds the per-user statistics cache |
| `mathgame-trim-recently-shown-problems` | daily 04:00 | `trim_recently_shown_problems` | caps each user's `recently_shown_problems` rows |
| `mathgame-watchdog` | every 5 min (`*:0/5`) | `deploy/watchdog.sh` | pages on sustained error patterns in the journal |

Timers are `Persistent=true` (a missed run while the box was down fires on
boot). The three jobs that must not overlap a manual run hold a `flock`
(`compress-events`, `check-disabled-videos`, `trim-recently-shown-problems`);
`update-statistics` does not.

## The build (`make`)

`make` (= `make all`) runs `build-api` → `build-cmds` → `build-web`.

| Target | What it does |
|---|---|
| `build-api` | regenerates `*_model.generated.go` / `*_handlers.generated.go` from `server/api/models.json` (Python codegen), `gofmt -s`, then builds `bin/apiserver` |
| `build-cmds` | depends on `build-api`; builds every `cmd/*` tool into `bin/` (see list below) |
| `build-web` | `frontend-conf`, then `npm install --force` + build into `web/build.next`, prettier, then swap `build.next` → `build` |
| `test` / `test-api` | `build-api` then `go test ./server/api` |
| `test-bundle-secrets` | rebuilds the web bundle against a canary config and fails if a secret leaks into `web/build` (the CI scan) |
| `test-all` | `test` + `test-bundle-secrets` — full local CI parity |
| `fmt` / `fmt-file` / `fmt-web` / `fmt-web-file` | canonical formatters — `gofmt -s` on the tree or a single Go file (`FILE=`), and `prettier --write` on `web/src` or a single web file (`FILE=`); single source of truth, invoked by `build-api` / `build-web` and the format-on-edit hook in `.claude/hooks/fmt-on-edit.sh` |
| `docs-check` | `scripts/docs_check.py`; pass `BASE=origin/master` to enforce per-area doc updates |
| `frontend-conf` | emits `web/src/conf.json` with only the public config fields |
| `check-bundle-secrets` | fails if a secret value from `$(CONF)` made it into `web/build` |
| `prod-api` / `prod-web` / `prod-maintenance` | the three service entrypoints |
| `clean` | drops test DBs, removes `bin/*`, generated Go, `swagger.yaml`, web build dirs; `go mod tidy` |

Two build subtleties worth knowing:

- **`build-web` never empties the live dir.** `react-scripts` wipes its output
  dir at the start of every build; building in place left `web/build` a bare
  directory listing for the whole `npm install` + webpack window while the old
  server kept serving it (#243). So it builds into `web/build.next` and swaps
  with two sub-millisecond renames; `serve` re-reads per request, so no restart
  is needed and a failed build (`set -e`) leaves `web/build` untouched
  (Makefile `build-web`).
- **`prod-web` fails loudly without TLS paths.** If `tls_cert_file` /
  `tls_key_file` are absent from `$(CONF)`, `serve` would silently fall back to
  plain HTTP on :443 and every HTTPS client sees the site as down — so the
  target asserts both are set before starting (Makefile `prod-web`).

Generated Go and migrations have their own rules — see `docs/schema.md`.
Never hand-edit `*.generated.go`.

## The deploy (`deploy/update.sh`)

Run from the repo root after fetching master:

```
git fetch origin master
git reset --hard origin/master
./deploy/update.sh
```

`update.sh` is idempotent (`set -euo pipefail`) and does, in order
(`deploy/update.sh`):

1. **`make`** — rebuild everything *before* touching any service. A failed
   build aborts here with the live site untouched (`build-web` swap semantics).
2. **Sync unit files** — `cp deploy/*.service` / `deploy/*.timer` to
   `/etc/systemd/system`, then `systemctl daemon-reload`.
3. **Start `mathgame-maintenance`** — its `Conflicts=` stops `mathgame-web`, so
   users see the maintenance page (HTTP 503, `Retry-After: 120`) through the
   disruptive window. If anything below fails, `set -e` exits with the
   maintenance page still up.
4. **`systemctl restart mathgame-api`** — the new binary boots and runs DB
   migrations on startup (`api.RunMigrations`, called from `cmd/apiserver/main.go`
   `main`).
5. **Restart the timers** (picks up any schedule change).
6. **Start `mathgame-web`** — its start stops the maintenance page (`Conflicts=`).

### When a generation/difficulty change is part of the deploy

`update.sh` does **not** run the problem-generation backfills — they are manual
and must be slotted into the disruptive window by hand when the change requires
them. The ordering and rationale are owned by `docs/problem-generation.md`; the
operational summary:

- After the new binary is built but **before** serving normally, with the
  maintenance page up:
  1. `./bin/recompute_problem_type_bitmap -config conf.json -dry-run` — read the
     lexer census, zero-bitmap, and unknown-rule review lists; then run it for
     real. The lone-letter `?` rewrite mutates stored expressions, so this runs
     **first**.
  2. `./bin/recompute_problem_difficulty -config conf.json` — restamps the
     difficulty column. **Must run after** the bitmap tool, because difficulty
     is computed from the post-rewrite expressions.
- Any change to `ComputeProblemDifficulty`'s output (the formula now lives in the
  shared `server/mathcore` kernel) **requires** a `DifficultyVersion` bump
  (otherwise `recompute_problem_difficulty` skips every
  row as already-at-version) plus the recompute run on deploy — per
  `docs/problem-generation.md`. The doc-sync test
  (`server/api/docs_sync_test.go`) blocks the version bump from landing
  undocumented.
- **Record the required steps, in order, at the bottom of the commit message** of
  the change that needs them — they carry into the PR body, so they aren't
  rediscovered from the diff during the deploy window.
- `revalidate_word_problems` is optional and costs one LLM call per WORD row —
  run it out-of-band, not in the deploy window.

### First-time provisioning

From-scratch setup of a new Ubuntu 22.04 host (Go 1.24.2). One-time; everything
else in this doc is ongoing operation.

**Toolchain:**

```
wget -c https://go.dev/dl/go1.24.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.2.linux-amd64.tar.gz   # put /usr/local/go/bin on PATH
sudo apt install make
sudo apt-get install nodejs npm
sudo npm install -g serve
```

**Database:**

MySQL 8.0+ (8.4 LTS recommended). The schema is `utf8mb4` throughout and the app
expects `utf8mb4_unicode_ci` as the default collation so string sort order is
consistent across hosts. Create the DB explicitly:

```sql
CREATE DATABASE mathgame CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Set these server-level defaults too, so a later `mysqldump` or a `CREATE TABLE`
that omits an explicit charset can't drift:

| Variable | Recommended |
|---|---|
| `character_set_server` | `utf8mb4` |
| `collation_server` | `utf8mb4_unicode_ci` |
| `sql_mode` | at least `NO_ENGINE_SUBSTITUTION` (strict mode is tolerated, not required) |

Check with `SHOW VARIABLES LIKE '<name>';`; where to set them depends on the host
(managed-host config UI, `my.cnf`, etc.). Migrations run on the first `apiserver`
start — see `docs/schema.md`.

**TLS cert (Let's Encrypt):**

```
sudo ln -s /snap/bin/certbot /usr/bin/certbot
sudo certbot certonly --standalone
```

**Clone, build, install units:**

```
git clone https://github.com/gdmen/mathgame_2.git && cd mathgame_2 && make
sudo cp deploy/*.service deploy/*.timer /etc/systemd/system && sudo systemctl daemon-reload
sudo systemctl enable mathgame-api mathgame-web
sudo systemctl enable --now mathgame-{compress-events,check-disabled-videos,update-statistics,trim-recently-shown-problems,watchdog}.timer
sudo service mathgame-api start && sudo service mathgame-web start
```

(`mathgame-maintenance` is started on demand by `update.sh`, never enabled.)

**`conf.json`** (gitignored): set `ntfy_topic` (an unguessable `ntfy.sh` topic,
subscribed in the ntfy app) and the TLS paths `tls_cert_file` / `tls_key_file`
(the Let's Encrypt `fullchain.pem` / `privkey.pem`) — both `prod-web` and the
maintenance page read them. Cert renewal: `certbot renew`, then restart
`mathgame-web`.

## The tools (`cmd/*`)

Every DB tool takes `-config` (default `conf.json`) and connects with the
`utf8mb4` / `parseTime` / UTC DSN. Tools that mutate take `-dry-run`.

### Servers / build artifacts

| Tool | Purpose | Notes |
|---|---|---|
| `apiserver` | the API server | reads `conf.json` from CWD (no `-config` flag); runs migrations on startup |
| `maintenance_server` | static 503 maintenance page | `-port` (default 443); serves HTTPS iff both TLS paths set, else plain HTTP; fails if only one is set (`main`, the "only one of tls_cert_file/tls_key_file" guard) |

### Scheduled maintenance jobs

| Tool | Flags | Purpose |
|---|---|---|
| `compress_events` | `-dry-run` | runs migrations, then `api.PlanCompress` to collapse event rows |
| `check_disabled_videos` | `--enable` | lists `disabled=1` videos, checks playability via YouTube Data API v3 with an oembed fallback; `--enable` writes `disabled=0` for playable ones |
| `update_statistics_cache` | `-user_id` (0 = all) | runs migrations, rebuilds the statistics cache |
| `trim_recently_shown_problems` | `-dry-run` | caps each user's `recently_shown_problems` to `recentlyShownProblemsTrimSize` (`generate_problems.go`) |

`make check-disabled-videos` / `make fix-disabled-videos` build and run
`check_disabled_videos` directly (the latter with `--enable`).

### Problem-generation backfills (see `docs/problem-generation.md`)

| Tool | Flags | Purpose |
|---|---|---|
| `recompute_problem_type_bitmap` | `-dry-run`, `-limit` | restamps `problem_type_bitmap` via the admission pipeline; SET (re-runnable); applies the lone-letter `?` rewrite; prints lexer/zero-bitmap/unknown-rule reports. Run **before** the difficulty tool. |
| `recompute_problem_difficulty` | `-dry-run`, `-limit` | restamps the `difficulty` column from `ComputeProblemDifficulty`; idempotent; skips rows already at `DifficultyVersion`. Run **after** the bitmap tool. |
| `revalidate_word_problems` | `-dry-run`, `-limit`, `-workers`, `-start-id`, `-prefilter` | re-stamps WORD rows' topic bits from the LLM validator (one call per row, cheap model at default effort); bitmap-only writes; resume with `-start-id`. **`-prefilter` (default `true`)** skips rows a quantity/cue heuristic (`needsValidation`, `main.go`) judges single-step with a safe stamp, so most rows never hit the LLM — pass `-prefilter=false` for a full sweep. |

### Diagnostics

| Tool | Flags | Purpose |
|---|---|---|
| `diagnose_generation` | `-bitmap`, `-target`, `-epsilon`, `-n`, `-model` | runs the real LLM generator for a fixed envelope+target and reports the computed-difficulty distribution, admission/envelope outcome, in-window count, and WORD `symbolic_expression` validity. Writes nothing; needs a live `openai_api_key` in `conf.json` (reads from CWD). `-model` overrides the generator default for model-tier A/B (#263). |
| `compare_generators` | `-cells`, `-samples`, `-seed`, `-mode` | informational heuristic_2.0 review (#283): reads the pool from a snapshot and reports LLM-offload + coverage gaps. `-mode=samples` (default) puts fresh heuristic_2.0 output beside stored heuristic_1.0/llm rows in the highest-volume cells; `-mode=matrix` instead prints a complete per-bitmap difficulty-coverage grid over every distinct symbolic bitmap (not just the volume-ranked top). Writes nothing; needs MySQL creds in `conf.json`. NOT a merge gate (the CI B-gate is). |
| `verify_migrations` | `-before-config`, `-after-config` | one-off consistency check across the video de-dup/remap migrations (a pre-migration DB vs. a migrated one); does not run migrations. |
| `clean_test_dbs` | `-config` (default `test_conf.json`) | drops `mathgame_test_*` databases; invoked by `make clean`. |

## The watchdog (`deploy/watchdog.sh`)

Fires every 5 minutes (`mathgame-watchdog.timer`). For each entry in `WATCHES`
it greps the last hour of the `mathgame-api` journal and pushes a phone
notification via `ntfy.sh` if the count crosses the entry's threshold — a
*sustained* count, because the retry/self-heal layers absorb transient blips
(#200). Currently one watch: the `OpenAI error` pattern, threshold 5
(`deploy/watchdog.sh` `WATCHES`).

Each watch is rate-limited to one page per hour via a stamp file in
`$STATE_DIR` (`/run` by default); the stamp is written only on a successful
`curl`, so a failed push retries next tick. The `ntfy_topic` comes from
`conf.json` and is effectively a shared secret — empty/absent makes the
watchdog a quiet no-op. To add a watch, append a
`slug|threshold|label|pattern` line (pattern may contain spaces, not `|`).

## Operating notes

- **Logs:** `journalctl -u <service> -b -f` (services are listed in The
  topology above).
- **Manual reset (destructive):** `deploy/drop.sql` is
  `drop database mathgame; create database mathgame;` — a full wipe; migrations
  rebuild the schema on the next `apiserver` start. Not part of any automated
  flow.
- **Secret scan before deploy:** `make test-bundle-secrets` (or `test-all`)
  reproduces the CI bundle scan locally against a canary config; `CONF`
  overrides the config the web build reads.
- **The maintenance page is the safety net:** because `update.sh` raises it
  first and `set -e` aborts on any later failure, a broken deploy leaves users
  on "down for maintenance," not on errors.

## Related files

- `deploy/update.sh` — the deploy script (build → maintenance → restart → web).
- `deploy/watchdog.sh` — journal watchdog.
- `deploy/*.service`, `deploy/*.timer` — systemd units.
- `deploy/mathgame-maintenance.service` — the `Conflicts=`/`After=` swap with web.
- `deploy/drop.sql` — destructive full-DB reset.
- `Makefile` — all build/test/prod targets.
- `cmd/apiserver/main.go` — `main` runs `api.RunMigrations` on API startup.
- `cmd/maintenance_server/main.go` — `Handler` (503 page), `main` (TLS guard).
- `cmd/recompute_problem_type_bitmap/main.go`, `cmd/recompute_problem_difficulty/main.go`,
  `cmd/revalidate_word_problems/main.go` — generation backfills (contract in
  `docs/problem-generation.md`).
- `cmd/diagnose_generation/main.go` — generation diagnostics.
- `cmd/compare_generators/main.go` — heuristic_2.0 vs heuristic_1.0/llm pool comparison (#283).

## Extension checklists

**Add a scheduled job:**
1. Write `cmd/<tool>/main.go` (take `-config`; `-dry-run` if it mutates).
2. Build it in the `make build-cmds` target.
3. Add `deploy/mathgame-<tool>.service` (`Type=oneshot`, absolute
   `bin/<tool>` path, `flock` if it must not overlap) and
   `deploy/mathgame-<tool>.timer` (`OnCalendar=`, `Persistent=true`).
4. Add both to the `SERVICES`/`TIMERS` arrays in `deploy/update.sh`.
5. On existing hosts, `systemctl enable --now mathgame-<tool>.timer` (new hosts
   pick it up from the First-time provisioning `cp deploy/*.timer` glob).
6. Update the timer/tool tables above.

**Add a watchdog alert:** append a `slug|threshold|label|pattern` line to
`WATCHES` in `deploy/watchdog.sh`; update the watchdog section here.
</content>
</invoke>
