---
name: audit-project-areas
description: Audit the Project Areas registry in README.md against the codebase — find unregistered areas, stale docs, broken globs, and orphan docs — and produce a worklist. Usage: /audit-project-areas.
disable-model-invocation: true
---
Check that the Project Areas registry in `README.md` is still current with the code, and produce a
worklist. Delegate the read-heavy scan to a subagent so this conversation stays clean.

Steps:
1. Read the "Project Areas" registry block in `README.md` (names, docs, types, globs).
2. Spawn a subagent to scan the repo and report, against that registry:
   - **Coverage gaps** — substantial code/subsystems NOT owned by any area's globs (candidate NEW
     areas worth registering + documenting).
   - **Broken globs** — globs that now match nothing (files moved/renamed/deleted).
   - **Orphan docs** — files under `docs/` not registered to any area.
   - **Stale docs** — for each area whose doc exists, whether its owned files changed in git more
     recently than the doc. Compare the latest commit touching any glob vs. the latest commit
     touching the doc (`git log -1 --format=%cI -- <paths>`); flag areas where code moved ahead.
3. Present the findings as a worklist of concrete next actions:
   - register + document a new area: `/document-project-area <name>`,
   - refresh a stale doc: `/document-project-area <name>`,
   - fix a broken glob / orphan doc: a registry edit.
   This skill is READ-ONLY — report and recommend; the user picks what to act on. Do not edit the
   registry or any doc here.
