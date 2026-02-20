# Progress page – design (adult-facing stats)

## Decisions from design questions

- **Audience:** Adult (parent/guardian) – more detail and oversight.
- **Route:** `/progress`, linked from the main nav (e.g. "Progress" next to "Adults").

---

## Metrics to show

| Metric | Source | Notes |
|--------|--------|--------|
| Total problems done | Count `solved_problem` events | Single number |
| Total time doing math | Sum `value` for `working_on_problem` (stored in ms) | Display as minutes/hours |
| Total time watching videos | Sum `value` for `watching_video` (stored in ms) | Display as minutes/hours |
| Time spent working as % | work_minutes / (work_minutes + video_minutes) | Percentage, e.g. "70% math, 30% video" |

---

## Data / backend

- **Existing:** `events` (timestamp, user_id, event_type, value); `problems` (id, difficulty, problem_type_bitmap). `SOLVED_PROBLEM` value = problem_id.
- **New API:** `GET /api/v1/progress/:user_id` (auth-protected) that returns:
  - **Totals:** total_problems_solved, total_work_minutes, total_video_minutes (and derived work_pct).
- **Handler:** New stats-specific file (e.g. `server/api/progress_handlers.go`) for this endpoint; register route in `init.go`.

---

## UI layout (suggested)

1. **Header:** "Progress" (or "Your progress").
2. **Summary row:** Total problems | Total math time | Total video time | Work %.
Styling: adult-oriented, clear labels, reuse existing SCSS patterns; add `progress.scss` for this page.

---

## Tech notes

- **Auth:** Same as settings/play: token + user from AppView; progress endpoint behind `userMiddleware`, scope to `user.Id`.
- **Files:** `web/src/progress.js`, `web/src/progress.scss`; backend: new stats-specific handler file (e.g. `progress_handlers.go`), route in `init.go`.
