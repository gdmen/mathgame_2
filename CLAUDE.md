# mathgame_2 — agent instructions

## The problem-generation system

The problem-generation system (problem-type bits, the difficulty formula and
ceiling, the lexer/normalizer/evaluator, selection masks and SQL, the
generators, the prompts) is documented in **`docs/problem-generation.md`**.

- **Read it before modifying any of these.**
- **Update it in the same PR as any behavior change.** A doc-sync test
  (`server/api/docs_sync_test.go`) fails CI when the doc's anchors disagree
  with the code, so version bumps and bit changes cannot land undocumented.
- New problem-type bits MUST follow the new-bit checklist in that doc.
- Any change to `ComputeProblemDifficulty`'s output requires a
  `DifficultyVersion` bump and a `recompute_problem_difficulty` run on
  deploy.

## Conventions

- Generated files (`*_model.generated.go`, `*_handlers.generated.go`) come
  from `server/api/models.json` via `make build-api` — edit the JSON and
  regenerate, never the output.
- Migrations (`server/api/migrations/N.sql`) are split on `;` by a simple
  runner: no semicolons inside SQL comments. Guard every statement so it is
  a no-op when re-run or when run on a fresh schema.
- UI work follows the `/style-guide` page (`web/src/style_guide.js`):
  existing tokens and documented patterns; no new untokenized values without
  adding them to the style guide first.
