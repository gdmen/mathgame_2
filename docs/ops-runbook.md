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

nginx owns the public ports and serves the web bundle itself; one long-running
unit sits behind it:

| Service | `ExecStart` | Role |
|---|---|---|
| `mathgame-api` | `make prod-api` (`apiserver`, `GIN_MODE=release`) | the API; runs migrations on startup. Plain HTTP on loopback (`127.0.0.1:<api_port>`, `cmd/apiserver/main.go`); nginx proxies `/api/` to it |

It restarts on failure (`Restart=always`, `RestartSec=1s`, burst-limited to 5
in 500s). nginx is a stock distro unit — not one of ours, not in `SERVICES`.

### The front door (`deploy/nginx/mikeymath.conf`)

nginx terminates TLS on :443, canonicalizes the host, serves the static
bundle, and proxies the API. It is synced to `/etc/nginx/conf.d/` by `update.sh` like the systemd
units, and the canonicalization is what makes `www.mikeymath.org` work: the
SPA sends `window.location.origin` as its Auth0 `redirect_uri`, so a session
that starts on a non-canonical host sends an origin Auth0 rejects and login
dead-ends before the page renders.

| Listener | Behavior |
|---|---|
| `:80`, both hostnames | 301 to `https://mikeymath.org$request_uri` (ACME challenge path excepted) |
| `:443`, `www.mikeymath.org` | 301 to `https://mikeymath.org$request_uri` |
| `:443`, `mikeymath.org` | `root /var/www/mathgame/build`; `/api/` proxied to `apiserver` on loopback |

The docroot is a **published copy**, not the repo's `web/build`: `update.sh`
copies the bundle there with the same staged two-rename swap `build-web` uses.
Serving from the repo would need world-traversal on `/home/ubuntu`, which
would let `www-data` — the most exposed account on the box — read the secrets
in `conf.json`; and a missing permission bit would surface as every request
404ing while `nginx -t` and the deploy both report success.

The serving contract on the apex, top to bottom:

- **Maintenance flag.** While `/var/www/mathgame/maintenance.on` exists, every
  request answers 503 with the branded page, `Retry-After: 120` and
  `Cache-Control: no-store`. `update.sh` creates the flag for the disruptive
  window and removes it at the end; a failed deploy (or failed post-deploy
  smoke check) leaves it up, and touching it by hand is the manual "take the
  site down" switch. Two config subtleties the contract test pins: the error
  page lives in a **named location** (`error_page 503 @maintenance`), because
  a URI-form `error_page` re-runs the server-level `if`s on its internal
  redirect and the flag would 503 the maintenance page itself into nginx's
  default error page; and `add_header ... always` (a bare `add_header` skips
  non-2xx/3xx statuses).
- **`/api/` is the API** (`location ^~ /api/`): proxied unmodified to
  `apiserver` on loopback plain HTTP, with `Host`, `X-Real-IP`,
  `X-Forwarded-For` and `X-Forwarded-Proto` set. The `proxy_pass` port is
  hardcoded in the config and must match `api_port` in the host's `conf.json`
  — `update.sh` preflights the match before building. A 300s
  `proxy_read_timeout` replaces nginx's 60s default because the synchronous
  last-resort WORD generation (`docs/problem-generation.md`) can legitimately
  run for minutes. This makes the API same-origin with the app: the bundle
  builds its API URL from `api_host` alone (see First-time provisioning), and
  the maintenance flag closes the API along with the site during the deploy
  window.
- **The React shell answers an enumerated route list** (`/login`, `/play`,
  `/settings`, `/progress`, `/admin`, `/pin/*`, `/admin/*` → `app.html`),
  matched non-strict and case-insensitive (`~*`, optional trailing slash) for
  parity with React Router. A new top-level route
  in `web/src/index.js` must be added to the config and the contract test or
  its deployed URL answers with a 404 status on any hard load (the page still
  renders, since the 404 body is the shell, so nothing visible flags it) —
  `server/api/web_routes_sync_test.go` parses both files and fails the merge
  on a missed route. A catch-all is
  deliberately absent — see the build subtleties below for why the landing/app
  split forces this shape.
- **Caching:** everything unhashed answers `Cache-Control: no-cache`
  (revalidate every load — a heuristically cached shell would request hashed
  bundles the deploy swap already deleted), while the content-hashed
  `/static/` tree is `immutable` for a year.
- **Everything else is files**: `try_files $uri $uri/ $uri.html =404` gives
  the landing at `/` (via `index`), extensionless statics by their `.html`
  (`/privacy`), and a real 404 for unknown paths (403 folded in, so bare
  directory URLs answer the same). The 404 body is `app.html`
  (`error_page 403 404 =404 /app.html`, with the no-cache header riding it via
  `always`), so the shell's own not-found route (`NotFound` in
  `web/src/index.js`) is the 404 page and the status stays 404. Two consequences
  worth knowing: no-JS clients (curl, link unfurlers) see the bare shell rather
  than not-found copy, which the 404 status covers for crawlers; and a
  signed-in account mid-setup gets its setup takeover at an unknown URL, the
  same as on any app route.
- **gzip is nginx's** (`gzip on` + types; the list carries both
  `application/javascript` and `text/javascript` because nginx's bundled
  `mime.types` switched the `.js` mapping in 1.21.5 and prod runs 1.18).
- **Security headers ride every response:** `X-Content-Type-Options: nosniff`,
  `X-Frame-Options: DENY`, and `Referrer-Policy: strict-origin-when-cross-origin`.
  nginx drops the inherited server-level `add_header` set in any location that
  declares its own, so the set is re-declared in `/static/` and `@maintenance`;
  the contract test asserts all three on the landing page, a `/static/` asset,
  and the maintenance 503 so an overriding location can't silently drop them.
  `server_tokens off` keeps the nginx version out of the `Server` header.
  `Strict-Transport-Security` and `Content-Security-Policy` are deliberately not
  set here yet — both are standalone commitments tracked separately.

Every behavior above is exercised by `make test-nginx`
(`scripts/nginx_contract_test.sh`), which runs this very file — not a copy —
against a fixture build dir; see the build table below.

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
| `build-api` | regenerates `*_model.generated.go` / `*_handlers.generated.go` from `server/api/models.json` and `web/src/enums.generated.js` from the Go enum blocks (Python codegen), `gofmt -s`, regenerates the Swagger spec (`build-docs`), then builds `bin/apiserver` |
| `build-cmds` | depends on `build-api`; builds every `cmd/*` tool into `bin/` (see list below) |
| `build-web` | `frontend-conf`, `npm ci --include=dev`, `landing-assets`, then a Vite build into `web/build.next` (`--outDir`, plus a 1024MB Node heap: building the swagger-ui chunk OOMs the prod host's default half-of-1GB heap — it needs the box's swap; `build.sourcemap: false` in `web/vite.config.js` keeps `.map` files out of the shipped bundle), prettier, the landing/app HTML swap (below), then swap `build.next` → `build`. `--include=dev` because npm reads `NODE_ENV=production` as `--omit=dev`, which would skip `vite` and `sass` and break the build. `npm ci` (not `npm install`) so the deployed bundle is built from exactly the lockfile the CI `npm audit` gate certifies, and so the install fails loudly instead of re-resolving. It reinstalls the whole dependency tree every run (a few seconds), so a deploy needs the network — as does the Go side for any modules new since the last deploy (the go-swagger toolchain included) |
| `landing-assets` | compiles `web/src/landing.scss` → `web/public/landing.css` and copies the landing's woff2 files into `web/public/fonts/`; both outputs are generated and gitignored. The landing is plain HTML with no React, so it can reach neither the app's bundle nor its JS `@fontsource` imports and needs its own copies. `landing.scss` pulls in `styles.scss` with `@use`, so the two surfaces still compile from one token source |
| `test` / `test-api` / `test-cmds` | `test` = `build-api` then both Go suites; `test-api` (`./server/api`) and `test-cmds` (`./cmd/...`) run one each without rebuilding, which is how the CI Go job invokes them after its own `build-api` step. Every suite needs the MySQL from `test_conf.json` |
| `web-deps` | `npm ci` in `web/` — lockfile-exact, and fails if `package.json` and the lockfile have drifted |
| `test-web` | `web-deps`, then the `web/src` vitest suite in one pass (`npm test` = `vitest run`). This is exactly what the CI web job runs |
| `test-bundle-secrets` | rebuilds the web bundle against a canary config and fails if a secret leaks into `web/build` (the CI scan) |
| `test-nginx` | the front-door contract (`scripts/nginx_contract_test.sh`): stages `deploy/nginx/mikeymath.conf` itself — substituting only ports, cert paths, content roots and the API upstream, with a loud failure both for a renamed pattern and for an unsubstituted prod path — and curl-asserts the full serving contract from The front door above against a fixture build dir and a stub API upstream. Needs an nginx binary (`brew install nginx`; CI installs it from apt) |
| `test-all` | `test` + `test-web` + `test-bundle-secrets` + `test-nginx` — full local CI parity |
| `fmt` / `fmt-file` / `fmt-web` / `fmt-web-file` | canonical formatters — `gofmt -s` on the tree or a single Go file (`FILE=`), and `prettier --write` on `web/src` or a single web file (`FILE=`); single source of truth, invoked by `build-api` / `build-web` and the format-on-edit hook in `.claude/hooks/fmt-on-edit.sh` |
| `docs-check` | `scripts/docs_check.py`; pass `BASE=origin/master` to enforce per-area doc updates |
| `build-docs` | regenerate the committed `server/docs/spec/swagger.yaml` from the `server/docs` annotations and validate it; `check-swagger` builds the pinned go-swagger into `bin/swagger`. See [docs/swagger.md](swagger.md) |
| `frontend-conf` | emits `web/src/conf.json` with only the public config fields |
| `check-bundle-secrets` | fails if a secret value from `$(CONF)` made it into `web/build` |
| `prod-api` | the API service entrypoint |
| `clean` | drops test DBs, removes `bin/*`, generated Go, web build dirs; `go mod tidy` |

Two build subtleties worth knowing:

- **The landing page is `index.html`; the React shell is `app.html`.** The
  marketing pages (`/`, `/privacy`) are static HTML so crawlers, link unfurlers,
  and no-JS readers get real content instead of an empty JS shell. nginx's
  `index` directive resolves `/` to `index.html`, so the only way to put a
  static page at `/` is to *be* `index.html` — hence the two renames at the end
  of `build-web`. The flip side is that the React shell cannot be `index.html`,
  which is why the front door rewrites **an enumerated route list** to
  `app.html` instead of a catch-all: with a catch-all, `/privacy` and every
  missing asset would collapse into the shell with a 200 (the old
  shell-with-a-200 behavior); without one, extensionless statics resolve via
  `$uri.html` and unknown paths are real 404s. The route list, its extension
  procedure, and the full serving contract live in The front door above.

- **`build-web` never empties the live dir.** The bundler wipes its output
  dir at the start of every build; building in place left `web/build` a bare
  directory listing for the whole install + bundle window while the old
  server kept serving it (#243). So it builds into `web/build.next` and swaps
  with two sub-millisecond renames; nginx opens files per request, so no
  reload is needed. Any failed step aborts the target before the swap runs, so
  `web/build` keeps serving the last good bundle; the staging dir is cleared
  before each build, so no earlier run's output can survive into the swap
  (Makefile `build-web`).

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

1. **Preflight, then `make`** — first checks `conf.json`'s `api_host` (the
   origin baked into the bundle) and `api_port` (must match the front door's
   `proxy_pass` target), then rebuilds everything *before* touching any
   service. A failed check or build aborts here with the live site untouched
   (`build-web` swap semantics).
2. **Sync unit files** — `cp deploy/*.service` / `deploy/*.timer` to
   `/etc/systemd/system`, then `systemctl daemon-reload`.
3. **Publish the bundle** — `web/build` to `/var/www/mathgame/build` with the
   same staged two-rename swap as `build-web`, plus `maintenance.html`.
4. **Sync the front door** — `mikeymath.conf` to `/etc/nginx/conf.d/`, then
   `nginx -t` and `systemctl reload nginx`. The validation runs *before* the
   window opens, so a config nginx rejects aborts with the site still up,
   serving, and the previously installed config restored (a rejected file
   must not linger in `conf.d/`, where the next reload — e.g. certbot's
   renewal hook — would load it).
5. **`touch` the maintenance flag** — opens the disruptive window: nginx
   answers every request with the maintenance page (HTTP 503,
   `Retry-After: 120`). If anything below fails, `set -e` exits with the flag
   still in place.
6. **`systemctl restart mathgame-api`** — the new binary boots and runs DB
   migrations on startup (`api.RunMigrations`, called from `cmd/apiserver/main.go`
   `main`).
7. **Restart the timers** (picks up any schedule change).
8. **Wait for apiserver, remove the flag, smoke-check** — a loopback poll
   waits for the API's unauthenticated 401 while startup migrations run, with
   the maintenance page still covering users (the flag only gates nginx);
   then the flag comes down and `curl` hits `/`, `/play`, and the API route
   through the real front door (`--resolve` to loopback). A failure re-raises
   the flag and exits non-zero, so a deploy that cannot serve ends on the
   maintenance page, never on "Update complete."

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

Node is the opposite case: nothing in the repo constrains it, so the host runs
whatever provisioning installed, and 22.04's `apt` default is several majors
behind. Pin it deliberately. `deploy/update.sh` reaches `build-web` through a
bare `make`, so this host compiles the same bundle CI does and must not lag the
`node-version` in the CI workflows; when they disagree, a green merge becomes a
failed deploy. The floor is 22.12, where `require()` of an ES module starts
working, which newer releases of the web toolchain depend on.

```
wget -c https://go.dev/dl/go<version>.linux-amd64.tar.gz          # any recent release, see go.dev/dl
sudo tar -C /usr/local -xzf go<version>.linux-amd64.tar.gz        # put /usr/local/go/bin on PATH
sudo apt install make
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash - # apt's own nodejs is far older
sudo apt-get install -y nodejs
sudo apt install nginx
```

Leave the distro default site in place for now — the TLS cert step below needs
it as the `:80` listener, and the front-door install step removes it.

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

Webroot, not `--standalone`: standalone spins up its own temporary listener on
:80, which nginx holds permanently. Issuance runs while the distro default site
still owns :80 and serves `/var/www/html` — the same webroot the front door's
ACME location serves after it takes over, so the recorded renewal config works
in both worlds. The cert must carry both hostnames: `www` serves its redirect
over TLS, so it needs a valid certificate of its own.

```
sudo ln -s /snap/bin/certbot /usr/bin/certbot
sudo certbot certonly --webroot -w /var/www/html \
  -d mikeymath.org -d www.mikeymath.org \
  --deploy-hook "systemctl reload nginx"
```

The deploy-hook is the point: nginx is the only process that terminates TLS
(`apiserver` sits behind it on loopback plain HTTP), and it reads the cert
files at startup, so every renewal needs the graceful reload. The hook makes
that automatic; nothing else on the box needs a restart when the cert rolls.

**Clone, install, first deploy:**

Write `conf.json` (below) after cloning; `update.sh` builds, publishes the
bundle, installs the front door, and starts the API, so the first deploy is
just the normal deploy.

```
git clone https://github.com/gdmen/mathgame_2.git && cd mathgame_2
sudo rm -f /etc/nginx/sites-enabled/default   # the :80 block replaces it; cert already issued
./deploy/update.sh
sudo systemctl enable mathgame-api
sudo systemctl enable --now mathgame-{compress-events,check-disabled-videos,update-statistics,trim-recently-shown-problems,watchdog}.timer
```

**`conf.json`** (gitignored): set `ntfy_topic` — an unguessable `ntfy.sh` topic,
subscribed in the ntfy app.

`api_host` is the API origin **as the browser sees it**, baked into the web
bundle by `make frontend-conf`: `https://mikeymath.org` in prod (same-origin
through the front door), `http://localhost:8080` in dev, where the bundle
talks to `apiserver` directly. `api_port` is the port `apiserver` listens on;
it never reaches the bundle, and it must match the front door's `proxy_pass`
target. Getting either wrong is a hard outage — every API call from the
deployed bundle fails — so `update.sh` preflights both against the front
door before building, `gen_frontend_conf.py` rejects an `api_host` that is
not a full origin, and the deploy smoke-checks an API route through the
front door.

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
dies first. It returns a copy of the config naming that database alongside the
handle: a test that feeds a config to tool code must pass the copy, since the one
it read from `test_conf.json` still names the base database. `make test-cmds`
covers these suites locally and in CI, so a tool regression fails the merge gate.

### Servers / build artifacts

| Tool | Purpose | Notes |
|---|---|---|
| `apiserver` | the API server | reads `conf.json` from CWD (no `-config` flag); runs migrations on startup |

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
- **The maintenance page is the safety net:** nginx serves it whenever
  `/var/www/mathgame/maintenance.on` exists, and `set -e` aborts `update.sh`
  with the flag still up on any failure, so a broken deploy leaves users on
  "down for maintenance," not on errors. The flag doubles as the manual
  switch: `sudo touch` it to take the site down, `sudo rm` it to come back.
- **Front-door changes:** `nginx -t` before `systemctl reload nginx`, always —
  `update.sh` does both for you, and `make test-nginx` covers the behavior
  ahead of the deploy.

## Related files

- `deploy/update.sh` — the deploy script (build → units → front door → flag up → restart → flag down).
- `deploy/nginx/mikeymath.conf` — the front door: TLS, host canonicalization, the static bundle, the maintenance flag.
- `deploy/maintenance.html` — the page nginx serves while the maintenance flag exists.
- `deploy/watchdog.sh` — journal watchdog.
- `deploy/*.service`, `deploy/*.timer` — systemd units.
- `deploy/drop.sql` — destructive full-DB reset.
- `Makefile` — all build/test/prod targets.
- `scripts/nginx_contract_test.sh` — the front-door contract tests (`make test-nginx`).
- `.github/workflows/test.yml` — CI: the Go suite against a real MySQL, the web vitest suite, the
  nginx front-door contract, and the vulnerability gates (`govulncheck ./...`,
  `npm audit --omit=dev --audit-level=moderate`).
  The npm gate is scoped to production deps; the whole `web/` tree audits clean on Vite.
- `.github/workflows/web-bundle-secrets.yml` — CI: the bundle secret scan.
- `cmd/apiserver/main.go` — `main` runs `api.RunMigrations` on API startup.
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
