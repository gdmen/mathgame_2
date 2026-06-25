---
name: regen
description: Regenerate the API model/handler code from server/api/models.json via make build-api. Manual trigger only.
disable-model-invocation: true
---

# Regenerate API code

The generated Go files in `server/api/` are produced from
`server/api/models.json` — **never hand-edit them** (a deny rule blocks it; see
`docs/schema.md`).

1. Edit `server/api/models.json` (the schema is the source of truth).
2. Run `make build-api` — runs the two `generate_*.py` codegen steps, `gofmt -s`,
   and rebuilds `bin/apiserver`.
3. Review the regenerated diff and run `make test-api`.

Note: `models.json` and `server/api/migrations/**` are owned by the **schema**
project area — update `docs/schema.md` in the same PR if the model changed.
