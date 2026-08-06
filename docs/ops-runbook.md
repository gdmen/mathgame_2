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
restarts the services. The Go compiler comes from `go.mod`, not from the host
(see First-time provisioning below).

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
| `mathgame-watchdog` | every 5 min (`*:0/5`) | `deploy/watchdog.sh` | pages on error patterns in the journal |

Timers are `Persistent=true` (a missed run while the box was down fires on
boot). The three jobs that must not overlap a manual run hold a `flock`
(`compress-events`, `check-disabled-videos`, `trim-recently-shown-problems`);
`update-statistics` does not.

## The build (`make`)

`make` (= `make all`) runs `build-api` → `build-cmds` → `build-web`.

| Target | What it does |
|---|---|
| `build-api` | regenerates `*_model.generated.go` / `*_handlers.generated.go` from `server/api/models.json` and `web/src/enums.generated.js` from the Go enum blocks (Python codegen), `gofmt -s`, then builds `bin/apiserver` |
| `build-cmds` | depends on `build-api`; builds every `cmd/*` tool into `bin/` (see list below) |
| `build-web` | `frontend-conf`, `npm ci --include=dev`, `landing-assets`, then build into `web/build.next`, prettier, the landing/app HTML swap (below), then swap `build.next` → `build`. `--include=dev` because npm reads `NODE_ENV=production` as `--omit=dev`, which would skip `react-scripts` and `sass` and break the build. `npm ci` (not `npm install`) so the deployed bundle is built from exactly the lockfile the CI `npm audit` gate certifies, and so the install fails loudly instead of re-resolving. It reinstalls the whole dependency tree every run (a few seconds), so a deploy needs the network |
| `landing-assets` | compiles `web/src/landing.scss` → `web/public/landing.css` and copies the landing's woff2 files into `web/public/fonts/`; both outputs are generated and gitignored |
| `test` / `test-api` / `test-cmds` | `test` = `build-api` then both Go suites; `test-api` (`./server/api`) and `test-cmds` (`./cmd/...`) run one each without rebuilding, which is how the CI Go job invokes them after its own `build-api` step. Every suite but `cmd/maintenance_server` needs the MySQL from `test_conf.json` |
| `web-deps` | `npm ci` in `web/` — lockfile-exact, and fails if `package.json` and the lockfile have drifted |
| `test-web` | `web-deps`, then the `web/src` jest suite in one pass (`CI=true`). This is exactly what the CI web job runs |
| `test-bundle-secrets` | rebuilds the web bundle against a canary config and fails if a secret leaks into `web/build` (the CI scan) |
| `test-all` | `test` + `test-web` + `test-bundle-secrets` — full local CI parity |
| `fmt` / `fmt-file` / `fmt-web` / `fmt-web-file` | canonical formatters — `gofmt -s` on the tree or a single Go file (`FILE=`), and `prettier --write` on `web/src` or a single web file (`FILE=`); single source of truth, invoked by `build-api` / `build-web` and the format-on-edit hook in `.claude/hooks/fmt-on-edit.sh` |
| `docs-check` | `scripts/docs_check.py`; pass `BASE=origin/master` to enforce per-area doc updates |
| `build-docs` / `dev-docs` | generate `swagger.yaml` from the `server/docs` annotations and validate it, and serve it locally; both need go-swagger (`check-swagger` installs it). See [docs/swagger.md](swagger.md) |
| `frontend-conf` | emits `web/src/conf.json` with only the public config fields |
| `check-bundle-secrets` | fails if a secret value from `$(CONF)` made it into `web/build` |
| `prod-api` / `prod-web` / `prod-maintenance` | the three service entrypoints |
| `clean` | drops test DBs, removes `bin/*`, generated Go, `swagger.yaml`, web build dirs; `go mod tidy` |

Three build subtleties worth knowing:

- **The landing page is `index.html`; the React shell is `app.html`.** The
  marketing pages (`/`, `/privacy`) are static HTML so crawlers, link unfurlers,
  and no-JS readers get real content instead of an empty JS shell. `serve`
  resolves `/` to whatever `index.html` is, so the only way to put a static page
  at `/` is to *be* `index.html` — hence the two renames at the end of
  `build-web`. `web/public/serve.json` rewrites **an enumerated list of app
  routes** (`/login`, `/play`, `/settings`, `/progress`, `/pin/*`, `/admin/*`)
  to `app.html`; a new top-level React route must be added there too or its
  deployed URL 404s (the `Switch` in `web/src/index.js` carries the same
  warning). A catch-all rewrite cannot coexist with static
  clean URLs: `serve` 14 checks the filesystem before rewrites only for paths
  with an extension, and its rewrite engine cascades each rule's output through
  the remaining rules, so `/privacy → /privacy.html → catch-all → app.html` no
  matter the order (verified against `serve` 14 and its `serve-handler`
  source). With no catch-all, extensionless statics resolve via `cleanUrls`
  (`/privacy` → `privacy.html`), unknown paths get a **real 404** served from
  the branded `404.html` (`serve-handler` picks up that filename natively), and
  a missing asset is now a clean 404 rather than the old
  shell-with-a-200 behavior. **`prod-web` must not pass `serve`'s
  `-s`/`--single` flag** — it would rewrite everything to `index.html` and
  serve the marketing page in place of the app.

- **`build-web` never empties the live dir.** `react-scripts` wipes its output
  dir at the start of every build; building in place left `web/build` a bare
  directory listing for the whole install + webpack window while the old
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
     **first**. A stamp-rule change (e.g. the answer-side NEGATIVES invariant)
     also requires this run — restamps read the stored answer, not just the
     expression.
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

### First-time provisioning

From-scratch setup of a new Ubuntu 22.04 host. One-time; everything else in
this doc is ongoing operation.

**Toolchain:**

The Go installed here is only a bootstrap — any release recent enough to
understand `go.mod`'s `toolchain` directive, and older than it. Under the
default `GOTOOLCHAIN=auto` the `go` command runs the newer of the two, fetching
the directive's toolchain when it has to, so keeping the bootstrap behind is
what leaves `go.mod` deciding which compiler builds prod. Consequences: the
first deploy after a `toolchain` bump needs the module proxy reachable
(`GOPROXY`, default `proxy.golang.org`); a bare `go version` reports the
bootstrap unless it is run inside the repo, so read a built binary with
`go version -m bin/apiserver` instead; and `GOTOOLCHAIN=local` on the host
breaks deploys outright.

```
wget -c https://go.dev/dl/go<version>.linux-amd64.tar.gz          # any recent release, see go.dev/dl
sudo tar -C /usr/local -xzf go<version>.linux-amd64.tar.gz        # put /usr/local/go/bin on PATH
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

`auth0_management_clientId` / `auth0_management_clientSecret` are the credentials
of an Auth0 machine-to-machine application authorized for the Management API with
the `delete:users` scope, and `auth0_management_domain` is the tenant's canonical
`<tenant>.<region>.auth0.com` (the Management API is not served on a custom
domain; leave it empty only if `auth0_domain` already *is* the canonical one).
Account deletion needs these to remove the Auth0 identity; with them unset it
still scrubs our database and only logs the skip, so the visible symptom of
forgetting them is orphaned Auth0 identities, not a failed deletion. Setting
exactly one of the two credentials logs an error on every deletion. See
[accounts.md](accounts.md).

## The tools (`cmd/*`)

Every DB tool takes `-config` (default `conf.json`) and connects with the
`utf8mb4` / `parseTime` / UTC DSN. Tools that mutate take `-dry-run` — with one
deliberate exception: `cleanup_unused_problems` deletes rows irreversibly, so it
inverts the convention and is dry-run **by default**, writing only under
`-apply`.

A tool's DB-backed test gets its database from `apitest.SetupTestDB`
(`server/api/apitest`): a throwaway `mathgame_test_<label>_N` database migrated
to the current schema, dropped on cleanup, and swept by `clean_test_dbs` if a run
dies first. `make test-cmds` covers these suites locally and in CI, so a tool
regression fails the merge gate.

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
| `recompute_problem_type_bitmap` | `-dry-run`, `-limit` | restamps `problem_type_bitmap` via the admission pipeline; SET (re-runnable); WORD rows with a `symbolic_expression` restamp from the skeleton (`WordFormBitmap`, matching the insert path); applies the lone-letter `?` rewrite; prints lexer/zero-bitmap/unknown-rule/skeleton-reject reports. Run **before** the difficulty tool. |
| `recompute_problem_difficulty` | `-dry-run`, `-limit` | restamps the `difficulty` column from `ComputeProblemDifficulty`; idempotent; skips rows already at `DifficultyVersion`. Run **after** the bitmap tool. |

### Manual cleanup (not scheduled)

| Tool | Flags | Purpose |
|---|---|---|
| `cleanup_unused_problems` | `-apply`, `-generator`, `-status` (default `deprecated`), `-keep-per-cell`, `-limit`, `-batch-size` | culls problem rows nothing references, to bound pool growth. **Dry-run by default** (reports candidates by generator/status + a sample); `-apply` deletes. Guard (no DB-level FKs): a row is a candidate only if referenced by none of `events` (problem-id-carrying types, dual-format extraction), `gamestates`, `review_queue`, `recently_shown_problems`. Policy: never-referenced rows whose status is in `-status` (default just `deprecated`; `reported`/`incorrect` kept unless named) are culled outright; never-referenced `active` rows are culled only past `-keep-per-cell` (default `SelectionPoolCap`) per (bitmap, difficulty) cell, so a cull never drops an active pool below the selection cap (`active` in `-status` is ignored). Does one full `events` scan (no `event_type` index) — **run off-peak.** Not a timer; run by hand when the pool needs trimming. |

### Development codegen (not deploy tools)

`make gen-difficulty-fixtures` regenerates the Go↔JS difficulty-band parity fixtures
(`web/src/difficulty_band_fixtures.json`, via `cmd/gen_difficulty_fixtures`) after a
ceiling/floor formula change; the fixture sync tests fail CI until it is run. Development-time
only — nothing on the host runs it.

### Diagnostics

| Tool | Flags | Purpose |
|---|---|---|
| `compare_generators` | `-cells`, `-samples`, `-seed`, `-mode` | informational heuristic_2.0 review (#283): reads the pool from a snapshot and reports LLM-offload + coverage gaps. `-mode=samples` (default) puts fresh heuristic_2.0 output beside stored heuristic_1.0/llm rows in the highest-volume cells; `-mode=matrix` instead prints a complete per-bitmap difficulty-coverage grid over every distinct symbolic bitmap (not just the volume-ranked top). Writes nothing; needs MySQL creds in `conf.json`. Counts only `status = 'active'` rows (the post-migration-46 successor to `disabled = 0`). NOT a merge gate (the CI B-gate is). |
| `clean_test_dbs` | `-config` (default `test_conf.json`) | drops `mathgame_test_*` databases; invoked by `make clean`. |

## The watchdog (`deploy/watchdog.sh`)

Fires every 5 minutes (`mathgame-watchdog.timer`). For each entry in `WATCHES`
it greps the last hour of the `mathgame-api` journal and pushes a phone
notification via `ntfy.sh` if the count crosses the entry's threshold. Usually
that means a *sustained* count, because the retry/self-heal layers absorb
transient blips (#200). Four watches (`deploy/watchdog.sh` `WATCHES`):

| Slug | Threshold | Pattern | Catches |
|---|---|---|---|
| `openai` | 5/hour | `OpenAI.*error.*after retries` | any OpenAI call that exhausted its retries, on either the narration or the validation path. Deliberately excludes the per-attempt `OpenAI transient error` warnings, which self-heal. |
| `openai-narrate-content` | 5/hour | `OpenAI narrate content error` | the narrator answered, but with something that is not the JSON list the prompt asks for (`NarrateProblems`). The whole batch is lost and those requests fall back to heuristic problems, the same user-visible degradation the `openai` watch pages for, hence the same threshold. A sustained count means prompt or model drift, not a blip. |
| `openai-quota-code` | 1/hour | `after retries.*openai_code=insufficient_quota` | the billing failure below, matched on OpenAI's machine-readable code rather than its prose. The `after retries` half is not redundant: it anchors the match to our own wording, so a model that echoes the code back inside a narration cannot page anyone. |
| `openai-quota` | 1/hour | `You exceeded your current quota` | the 429 sub-type that does *not* self-heal: out of credits, spending cap, or a dead payment method. Every LLM call falls back to the heuristic generator until billing is fixed, so no word problems are served. The system is degraded rather than broken, which is exactly why it needs its own page. Titled `BILLING` to be scannable on a lockscreen. |

A quota outage trips the generic watch too, since its pattern also matches
those lines; the quota ones cross their threshold sooner (1 vs 5). All send
`Priority: high`; the `BILLING` title, not the ntfy priority, is what
separates them on the lockscreen.

The content error gets its own slug rather than a wider `openai` pattern for
two reasons. The call succeeded, so it is not an after-retries failure, and
folding it in would make that pattern's wording a lie. Loosening the pattern
toward a bare `OpenAI.*error` is also exactly what the `after retries`
requirement exists to prevent: the prompt and content-error lines put
model-authored text in the journal, and a pattern the model can match is a
pattern the model can page you with. A separate slug also gets its own
threshold, cooldown and notification title, so the page says which failure
fired.

Notification bodies are truncated to 300 characters per line (`cut -c`, which
counts bytes on GNU coreutils). The content-error line carries the model's
entire response, and ntfy rejects an oversized body, which would fail the
`curl`, skip the stamp, and retry every 5 minutes forever.

Each watch is rate-limited to one page per hour via a stamp file in
`$STATE_DIR` (`/run` by default); the stamp is written only on a successful
`curl`, so a failed push retries next tick. The `ntfy_topic` comes from
`conf.json` and is effectively a shared secret — empty/absent makes the
watchdog a quiet no-op. To add a watch, append a
`slug|threshold|label|pattern` line (pattern may contain spaces, not `|`).

Smoke-test a watch on the host without waiting for a real failure. `journalctl
-u` matches the *unit* a message came from, not a `logger` tag, so a synthetic
line has to be logged by a real unit. `UNIT` and `STATE_DIR` are overridable
for exactly this, which keeps the test off `mathgame-api` and off the live
cooldown stamps in `/run`:

```
sudo systemd-run --unit=mathgame-smoke --collect /bin/echo \
  "OpenAI narrate error after retries: openai_code=insufficient_quota: error, status code: 429, status: Too Many Requests, message: You exceeded your current quota, please check your plan and billing details."
sudo env UNIT=mathgame-smoke STATE_DIR=/tmp deploy/watchdog.sh /home/ubuntu/mathgame_2/conf.json
```

That pages for real, so expect the notifications on your phone: that one line
trips both quota watches. To check a
pattern without sending anything, count it against the live journal instead:

```
journalctl -u mathgame-api --since "1 hour ago" --no-pager | grep -c "You exceeded your current quota"
```

Every pattern is matched against log *text*, which drifts from two directions.
A reworded `glog` line in `server/llm_generator` breaks the watches that key on
our own wording, and those are at least visible in our own diffs. The
`openai-quota` pattern is matched against OpenAI's English prose, which nothing
in CI can catch them rewording.

The two quota watches are a transition, deliberately overlapping. The Go
client's `Error()` string prints only `status code` / `status` / `message` and
drops the machine-readable code, so `chatCompletionWithRetry` now prefixes
`openai_code=<code>` onto the error it returns (`retry.go`
`withOpenAIErrorCode`); both call sites log that error, so the code reaches the
journal on the final error line. The prose watch stays as the fallback: a
journal window spanning a deploy still holds code-less lines from the old
binary, an error can arrive with no code at all, and the per-attempt transient
warnings log the raw error either way. The cost is that a real quota outage
pages twice, which is the right side to err on for a billing outage. The prose
pattern is also the model-triggerable one, since a narration quoting OpenAI's
sentence matches it while the anchored code pattern rejects it. Once prod
journals show the code appearing reliably, drop the `openai-quota` entry.

Note the prose count overstates distinct failures. 429s are retryable, so one
failed call logs three per-attempt warnings, the final error, and the caller's
re-log of that same error in `generate_problems.go`: five lines carrying the
quota sentence, of which `openai-quota-code` counts the one final error line.
The prose pattern is deliberately left broad rather than anchored to the final
error, because at threshold 1 a missed match costs more than an inflated count.

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
- `.github/workflows/test.yml` — CI: the Go suite against a real MySQL, the web jest suite, and the
  vulnerability gates (`govulncheck ./...`, `npm audit --omit=dev --audit-level=moderate`).
  The npm gate is scoped to production deps: the dev-tree findings are react-scripts', unfixable
  by any upgrade, and knowingly accepted until the CRA-to-Vite migration (#382) retires it.
- `.github/workflows/web-bundle-secrets.yml` — CI: the bundle secret scan.
- `cmd/apiserver/main.go` — `main` runs `api.RunMigrations` on API startup.
- `cmd/maintenance_server/main.go` — `Handler` (503 page), `main` (TLS guard).
- `cmd/recompute_problem_type_bitmap/main.go`, `cmd/recompute_problem_difficulty/main.go` —
  generation backfills (contract in `docs/problem-generation.md`).
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
