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
| Gameplay loop & companion | [docs/gameplay.md](docs/gameplay.md) | ✅ |
| Settings & envelope | [docs/settings.md](docs/settings.md) | ✅ |
| Accounts, access & onboarding | [docs/accounts.md](docs/accounts.md) | ✅ |
| Design system | [web/src/style_guide.js](web/src/style_guide.js) | ✅ (the `/style-guide` page is the living reference) |
| Data model & schema | [docs/schema.md](docs/schema.md) | ✅ |
| Deploy & ops runbook | [docs/ops-runbook.md](docs/ops-runbook.md) | ✅ |

The block below is the machine-readable registry (parsed by `scripts/docs_check.py`); the
`documenter` agent maintains it. `name → doc → type (anchored\|prose) → globs (owned files)`.

<!-- BEGIN PROJECT-AREA REGISTRY (parsed by scripts/docs_check.py) -->
```
problem-generation  doc=docs/problem-generation.md  type=anchored
  globs: server/mathcore/**, server/api/stamping.go, server/llm_generator/**, server/generator/**
generator-versions  doc=docs/generator-versions.md  type=anchored
  globs: server/generator/heuristic2.go, server/llm_generator/generate_problem.go
selection  doc=docs/selection.md  type=anchored
  globs: server/api/generate_problems.go, server/api/generator_rank.go, server/api/select_lru.go, server/api/pool_supply.go, server/api/trim_recently_shown.go
adaptive-difficulty  doc=docs/adaptive-difficulty.md  type=anchored
  globs: server/api/process_events.go, server/api/topic_stats.go, server/api/spaced_repetition.go
events  doc=docs/events.md  type=anchored
  globs: server/api/event_types.go, server/api/event_compress.go, server/api/statistics_handlers.go
videos  doc=docs/videos.md  type=anchored
  globs: server/api/youtube.go
gameplay  doc=docs/gameplay.md  type=prose
  globs: web/src/play.js, web/src/problem.js, web/src/video.js, web/src/companion.js
settings  doc=docs/settings.md  type=anchored
  globs: web/src/settings.js, web/src/bitmap_validation.js
accounts  doc=docs/accounts.md  type=prose
  globs: server/api/roles.go, web/src/auth0.js, web/src/pin.js, web/src/setup.js
design-system  doc=web/src/style_guide.js  type=prose
  globs: web/src/styles.scss, web/src/components.scss
schema  doc=docs/schema.md  type=anchored
  globs: server/api/models.json, server/api/migrations/**
ops-runbook  doc=docs/ops-runbook.md  type=prose
  globs: deploy/**, Makefile, cmd/**
```
<!-- END PROJECT-AREA REGISTRY -->

# Local development

## Prerequisites
- Node.js
- Go 1.24.2 (see `go.mod`)
- MySQL 8.0+ (8.4 LTS recommended)
- go-swagger — optional, only for `make build-docs` / `make dev-docs`

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
