---
name: difficulty-bump
description: Surfaces the checklist for a difficulty-formula change. Use whenever ComputeProblemDifficulty's output changes.
---

# Difficulty formula change

Two docs own everything here — follow them; this skill only routes you to them:

- **What to change:** `docs/problem-generation.md` (Problem-generation area) — the
  `DifficultyVersion` bump, the reference-value update, and the recompute
  sequence. Pinned to the code by `docs_sync_test` / `make docs-check`.
- **Deploy:** `docs/ops-runbook.md` → "When a generation/difficulty change is
  part of the deploy" — the ordered recompute commands and the rule to record
  them at the bottom of the commit message.
