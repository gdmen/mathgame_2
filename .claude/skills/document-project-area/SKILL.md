---
name: document-project-area
description: Create or update one project area's source-of-truth doc by running the documenter agent. Usage: /document-project-area <area> [--compare]. Areas are listed in the Project Areas registry in README.md.
disable-model-invocation: true
---
Create or update the source-of-truth doc for ONE project area, using the `documenter` subagent
(it reads the area's code in its own context and writes the doc, keeping this conversation clean).

`$ARGUMENTS` = an area name, optionally `--compare` (derive + diff, write nothing) or `--all`.

Steps:
1. Read the "Project Areas" registry block in `README.md`.
2. Resolve the area:
   - **Existing area** (name is in the registry): use its row (doc, type, globs).
   - **New area** (not registered): first propose a registry row — `name`, `doc` path
     (`docs/<name>.md`), `type` (`anchored` if it has code constants/enums worth pinning, else
     `prose`), and `globs` (the files it owns) — and get the user's OK. Add the row to the README
     registry (and the human table), then continue.
   - Unknown and not clearly new: list the registry's area names and stop.
3. Spawn the `documenter` subagent for the area, passing its registry row and whether this is a
   `--compare` run. (`--all`: run each registry area in sequence, reviewing each.)
4. Relay the documenter's report — what changed, code↔doc / doc↔doc mismatches, anchor status.
   Normal run: confirm the doc was written and (for a new area) the README row added. `--compare`:
   present the proposed doc + diff; write nothing.
5. Run `make docs-check` (registry integrity). If the doc is `anchored`, make sure
   `server/api/docs_sync_test.go` asserts its anchors, then `go test ./server/api/ -run DocsSync`.

Use area names ONLY from the registry — never invent one.
