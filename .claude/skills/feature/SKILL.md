---
name: feature
description: The standard new-feature workflow for mathgame_2 — explore→research→design (plan), TDD implement, review, test, and ship a draft PR. Use when starting any non-trivial feature or change.
---

# Feature workflow

The repeatable loop for a new feature. Stages are deliberately gstack-free in
the spine; reach for optional helpers (`/spec`, `/investigate`,
`/plan-eng-review`) only when they pay off.

## 1. Plan (explore → research → design)

Never design from memory.

- **Orient — the entry point is `docs/`, not code.** Open the **Project areas
  registry at the top of `README.md`**, find the area(s) this feature touches,
  and read each area's single source-of-truth doc (e.g.
  `docs/problem-generation.md`, `docs/selection.md`, `docs/schema.md`). Read root
  `CLAUDE.md` too. `README.md`'s "Local development" section is only for how to
  build/run.
- **Explore the code.** From those docs, follow into the affected subsystem
  (`server/api/`, `server/generator/`, `server/llm_generator/`, or `web/src/`).
  Use Explore subagents so the file-reading stays out of the main context and
  returns a summary.
- **Research.** Web-search for (a) how others have solved similar problems and
  (b) current best practices for the approach; plus authoritative docs for any
  unfamiliar library/API. For LLM/Anthropic work, consult the `claude-api`
  skill. Look for existing helpers/patterns to reuse before writing new code.
- **Design.** In plan mode, write the implementation plan. Use `/spec` first when
  intent is fuzzy. Branch off freshly-pulled master before implementing.

## 2. Implement (TDD — red → green → refactor)

- **Red:** write a failing test first that captures the new behavior. Go tests
  in `server/api/` (table-driven to match the package); `make test-api` is the
  inner loop. Confirm it fails for the right reason before writing code.
- **Green:** the minimum code to pass. **Never edit `*.generated.go`** — edit
  `server/api/models.json` and run `make build-api` (a deny rule blocks direct
  edits anyway).
- **Refactor:** clean up with tests green (KISS/DRY); `/simplify` for a focused
  cleanup pass.
- **Docs in the same PR.** If you changed an area's owned files, update that
  area's doc (`make docs-check BASE=origin/master` enforces it; `docs_sync_test`
  pins anchored docs to code constants). Keep the doc the single source of truth
  — don't restate what it owns in this skill, a comment, or a note; link to it
  (see CLAUDE.md). Only what the doc doesn't own (e.g. a workflow step) belongs
  elsewhere.

## 3. Review

`/code-review` — raise the effort, or use `ultra` for risky changes. `/simplify`
for a cleanup-only pass.

## 4. Test

`make test-all` (Go tests + web bundle-secret scan; full CI parity) as the final
gate beyond the TDD inner loop.

## 5. Ship

Commit → push → open a **draft PR via the GitHub MCP** directly (not `/ship`),
following the CLAUDE.md PR rules (branch off master, draft PR, KISS/DRY, outside
review before commit). Deploy is handled manually.
