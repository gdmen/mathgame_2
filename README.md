# Project areas

This repo is organized into **project areas**, each with a single source-of-truth doc. **Update
an area's doc in the same PR as a behavior change** — `make docs-check` enforces that an area's
doc is touched when its code changes, and `docs_sync_test` pins the `anchored` docs to code
constants. Create or refresh an area's doc with `/document-project-area <area>`; audit the
registry's currency against the code with `/audit-project-areas`.

| Area | Doc | Status |
|------|-----|--------|
| Problem generation & difficulty | [docs/problem-generation.md](docs/problem-generation.md) | ✅ |
| Generator versions (provenance) | [docs/generator-versions.md](docs/generator-versions.md) | ✅ |
| Selection & serving | [docs/selection.md](docs/selection.md) | ✅ |
| Adaptive difficulty & progression | [docs/adaptive-difficulty.md](docs/adaptive-difficulty.md) | ✅ |
| Events & analytics | [docs/events.md](docs/events.md) | ✅ |
| Reward videos & playlists | [docs/videos.md](docs/videos.md) | ✅ |
| Gameplay loop | [docs/gameplay.md](docs/gameplay.md) | ✅ |
| Settings & envelope | [docs/settings.md](docs/settings.md) | ✅ |
| Accounts, access & onboarding | [docs/accounts.md](docs/accounts.md) | ✅ |
| Design system | [web/src/style_guide.js](web/src/style_guide.js) | ✅ (the `/style-guide` page is the living reference) |
| Data model & schema | [docs/schema.md](docs/schema.md) | ✅ |
| Swagger API spec | [docs/swagger.md](docs/swagger.md) | ✅ |
| Deploy & ops runbook | [docs/ops-runbook.md](docs/ops-runbook.md) | ✅ |

The block below is the machine-readable registry (parsed by `scripts/docs_check.py`); the
`documenter` agent maintains it. `name → doc → type (anchored\|prose) → globs (owned files)`.

<!-- BEGIN PROJECT-AREA REGISTRY (parsed by scripts/docs_check.py) -->
```
problem-generation  doc=docs/problem-generation.md  type=anchored
  globs: server/mathcore/**, server/api/generation_funnel.go, server/llm_generator/**, server/generator/**
generator-versions  doc=docs/generator-versions.md  type=anchored
  globs: server/generator/heuristic2.go, server/llm_generator/generate_problem.go
selection  doc=docs/selection.md  type=anchored
  globs: server/api/generate_problems.go, server/api/generator_rank.go, server/api/select_lru.go, server/api/trim_recently_shown.go
adaptive-difficulty  doc=docs/adaptive-difficulty.md  type=anchored
  globs: server/api/process_events.go, server/api/spaced_repetition.go
events  doc=docs/events.md  type=anchored
  globs: server/api/event_types.go, server/api/event_compress.go, server/api/statistics_handlers.go, server/api/event_types_js_sync_test.go
videos  doc=docs/videos.md  type=anchored
  globs: server/api/youtube.go
gameplay  doc=docs/gameplay.md  type=prose
  globs: web/src/play.js, web/src/problem.js, web/src/video.js
settings  doc=docs/settings.md  type=anchored
  globs: web/src/settings.js, web/src/bitmap_validation.js, web/src/enums.generated.js
accounts  doc=docs/accounts.md  type=prose
  globs: server/api/roles.go, server/api/self_access.go, web/src/api.js, web/src/auth0.js, web/src/pin.js, web/src/setup.js
design-system  doc=web/src/style_guide.js  type=prose
  globs: web/src/styles.scss, web/src/components.scss
schema  doc=docs/schema.md  type=anchored
  globs: server/api/models.json, server/api/migrations/**
swagger  doc=docs/swagger.md  type=prose
  globs: server/docs/**
ops-runbook  doc=docs/ops-runbook.md  type=prose
  globs: deploy/**, Makefile, cmd/**, scripts/nginx_contract_test.sh
```
<!-- END PROJECT-AREA REGISTRY -->

# Issue labels

Every issue gets exactly one **kind** and one **priority**. **Modifiers** are optional and stack.
Areas are not labels: the project-area registry above owns the file-to-area mapping.

| Kind (pick one) | Meaning |
|---|---|
| `bug` | existing behavior is wrong |
| `feature` | new or changed behavior |
| `tech debt` | internal quality, no user-visible change (includes missing tests) |
| `documentation` | doc-only, no code |
| `spike` | investigate and decide; the deliverable is a decision, not a diff |

| Priority (pick one) | Meaning |
|---|---|
| `p1` | harming users or blocking other work; next up |
| `p2` | real, scheduled |
| `p3` | someday, opportunistic |

| Modifier (optional, any number) | Meaning |
|---|---|
| `security` | auth, secrets, headers, dependency advisories, anything an attacker can reach |
| `performance` | latency, query cost, payload size |
| `reliability` | correctness under failure: timeouts, retries, data integrity |
| `operations` | deploy, host, or CI; needs prod access or a deploy window |

`dependencies`, `go`, `javascript`, and `github_actions` are applied by Dependabot to PRs. Don't
hand-apply them. Duplicate and won't-do issues use GitHub's close reason, not a label.

# Local development

## Prerequisites
- Node.js 22.12+ (CI pins `22`, which resolves to the latest 22.x)
- Go per `go.mod` (the `toolchain` directive sets the minimum patch release; `GOTOOLCHAIN=auto` fetches it)
- MySQL 8.0+ (8.4 LTS recommended)

## Setup
1. **Config** — copy `conf.json_` to `conf.json` and fill in MySQL user/pass and any Auth0 / OpenAI keys you need.
2. **Database** — the schema is `utf8mb4` throughout and expects `utf8mb4_unicode_ci`, so create the DB explicitly:
   ```sql
   CREATE DATABASE mathgame CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   ```
   For a local install you may also need `ALTER USER 'root'@'localhost' IDENTIFIED BY '<password>';`.
   Migrations run automatically on apiserver startup ([docs/schema.md](docs/schema.md)). Server-level
   charset defaults and prod host provisioning are in the [ops runbook](docs/ops-runbook.md).
3. **Build** — `make` (use `make clean && make` to refresh after a long break).

## Run
Run from the repo root; the API reads `conf.json` from the current directory.

| Command | What it does |
|---|---|
| `make dev-api` | API server |
| `make dev-web` | web dev server |
| `make test` | run tests |

## Production
Build, deploy, ops, first-time host provisioning, and the destructive DB reset
(`deploy/drop.sql`) are all in the [ops runbook](docs/ops-runbook.md).
