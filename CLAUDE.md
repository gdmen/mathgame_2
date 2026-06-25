# mathgame_2 — agent instructions

## Project areas & docs

The repo is mapped into **project areas**, each with a single source-of-truth doc, in the
**Project Areas registry in `README.md`**. Before working on an area, read its doc; update that
doc in the same PR as a behavior change. Enforcement: `make docs-check BASE=origin/master` fails
when an area's owned files change without its doc being touched, and `server/api/docs_sync_test.go`
fails CI when an `anchored` doc's anchors disagree with the code — so version bumps, new bits, and
new migrations cannot land undocumented. Create or refresh a doc with the `documenter` agent
(`/document-project-area <area>`).

## Conventions

- **Never hand-edit generated files** (`*_model.generated.go`, `handlers.generated.go`): edit
  `server/api/models.json` and run `make build-api`. See `docs/schema.md`.
- **Migrations** (`server/api/migrations/N.sql`) must be re-run-safe and split cleanly on `;`
  (no semicolons inside SQL comments). See `docs/schema.md`.
- UI work follows the `/style-guide` page (`web/src/style_guide.js`): existing tokens and
  documented patterns; no new untokenized values without adding them to the style guide first.
- **Comments carry no issue refs and no references to removed code/features.** The area's doc is
  the only source of truth: don't restate what it owns anywhere else — a comment, a skill, a
  README note — link to the doc instead. A `// same as elsewhere`-style restatement is admitting
  duplication; extract a helper or file a follow-up.
- **Build through the Makefile, and check staged files before committing.** A bare
  `go build ./cmd/X/` without `-o` followed by `git add -A` sweeps a stray binary into the commit;
  use the Makefile targets and scan `git status` for build artifacts first.
