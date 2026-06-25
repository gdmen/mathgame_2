---
name: new-bit
description: Adding a new problem-type bit to mathgame_2. Walks the full new-bit checklist in docs/problem-generation.md so no site is missed. Use when adding a problem type / feature bit.
---

# Add a new problem-type bit

**The "new-bit checklist" in `docs/problem-generation.md` is the authoritative
source — open it and work every step top to bottom** (don't rely on memory; the
doc is maintained against the code). It spans the whole generation system; the
easy-to-miss gates worth double-checking by name:

- **DifficultyVersion bump** — a new bit shifts difficulty; follow
  `/difficulty-bump`.
- **Reachability** — per-problem exclusivity rules must be added at every site
  the doc lists, together.
- **Update the doc itself** — `docs_sync_test` / `make docs-check` fail CI if its
  `bits` anchor disagrees with the code.
- **Backfill** — re-run `recompute_problem_type_bitmap` if legacy rows need
  re-stamping.

Finish with `make test-all`.
