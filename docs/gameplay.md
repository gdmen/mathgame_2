# The gameplay loop (frontend)

The kid-facing play surface and its read-only adult mirror: how a problem is fetched, rendered,
answered, and how a reward video plays — plus the client-side event reporting that drives the
server's adaptive loop. **Change this doc in the same PR as any behavior change here**;
`make docs-check BASE=origin/master` fails when the owned files (`web/src/play.js`,
`web/src/problem.js`, `web/src/video.js`, `web/src/companion.js`) change without this doc.

This area is `type=prose` — it owns React view code, not pinned constants, so there is no doc-sync
anchor block. The doc stops at the HTTP boundary: what the client sends and what it expects back.
The server-side counterpart (event processing, gamestate mutation, problem selection) lives in
`server/api` and is covered by `docs/problem-generation.md` and the event-processing area.

## The model

Two routes render the same loop from a shared **gamestate** — the server's per-user cursor
(`Gamestate`, `server/api/gamestate_model.generated.go`): `{ user_id, problem_id, video_id, solved,
target }`. The loop's central branch is identical on both surfaces:

```
gamestate.solved >= gamestate.target  ?  show the reward video  :  show the problem
```

`solved` / `target` also drive the progress-meter width (`ProblemView`,
`ProblemCompanionView`).

| Route | View | File | Audience |
|---|---|---|---|
| `/play` | `PlayView` | `web/src/play.js` | the kid — interactive, mutates state |
| `/companion/:student_id` | `CompanionView` | `web/src/companion.js` | an adult, PIN-gated — read-only mirror |

Both are mounted by `MainView` in `web/src/index.js`.

### PlayView data flow (`/play`)

1. **Fetch.** On mount, GET `/play/:user.id` returns `{ gamestate, problem, video }` (`PlayView`;
   server shape `PlayData`, `server/api/meta_models.go`). A 403 redirects to `/` — the
   "add a video first" gate, where `customGetPlayData` returns Forbidden when the user has no
   enabled video. Empty / invalid bodies are logged and swallowed.
2. **Render LaTeX.** `problem.expression` is run through `PreprocessExpression` and rendered to an
   HTML string with KaTeX (`PlayView`, the `renderLatex` effect). A render failure posts a
   `bad_problem_system` event and swaps in the server-supplied replacement problem; if the server
   returns no replacement, it reloads `/play`. A corrupt expression self-heals without a visible
   error.
3. **Answer.** `ProblemView` holds the answer input; submitting (Enter or the button) posts
   `answered_problem` with the typed string.
4. **Advance.** The `answered_problem` response carries a fresh `{ gamestate, problem, video }`,
   which `PlayView` swaps in (the `eventReporter` callback, on the `answered_problem` branch),
   re-rendering the next problem — or the video, once `solved >= target`.

### CompanionView data flow (`/companion/:student_id`)

The mirror reads the same data through the generic GET-only REST endpoints rather than `/play`, so
it never mutates anything: `getGamestate` → `/gamestates/:student_id`, `getProblem` →
`/problems/:problem_id` (rendered through the same KaTeX path and exposing the **correct answer** —
an adult-only affordance), `getVideo` → `/videos/:video_id`, `getEvents` →
`/events/:student_id/3000` (filtered to the current problem — see Attempt reconstruction). A
`RefresherSingleton` re-polls gamestate and events on a fixed interval while the tab is focused;
access is PIN-gated by `RequirePin`.

## Event types reported from the client

Every event is POSTed to `/events` as `{ event_type, value }` with `value` stringified
(`genPostEventFcn`, `web/src/index.js`). Event-type strings are bare literals on the client and
must match the server constants in `server/api/enums.go` exactly. The table is the subset this area
emits.

| Event | Value | Emitted by | When |
|---|---|---|---|
| `working_on_problem` | interval ms | `EventReporterSingleton` ticker | every interval while focused and a problem is shown |
| `answered_problem` | typed answer string | `AnswerTracker.reportAnswer` | submit |
| `watching_video` | elapsed delta ms | `VideoView` `onProgress` | during playback |
| `done_watching_video` | video id | `VideoView` `onEnded` | video finishes |
| `error_playing_video` | error | `VideoView` `onError` | playback error |
| `bad_problem_system` | `{problem_id, explanation}` JSON | `PlayView` `renderLatex` catch | KaTeX throws |
| `bad_problem_user` | `{problem_id, explanation}` JSON | `PlayView` `handleReportSubmit` | adult reports a bad problem |

`done_watching_video` and `error_playing_video` both navigate back to `/play`, forcing a full
reload and a fresh gamestate fetch. The server also defines `logged_in`, `selected_problem`,
`solved_problem`, and the `set_*` settings events — none are emitted from this area.

## The reporting singletons

Two focus-gated loops keep traffic off backgrounded tabs:

- **`EventReporterSingleton`** (`web/src/play.js`) — a `Set` of "sticky" event types re-POSTed
  every `interval` ms (`conf.event_reporting_interval`). `working_on_problem` is the only sticky
  member: `ProblemView` adds it while a problem is shown and `AnswerTracker` removes it on submit,
  so "time on problem" accrues only while the kid is actually looking at one. The ticker is a no-op
  while the window is blurred.
- **`RefresherSingleton`** (`web/src/companion.js`) — the companion's read-poll, same focus/blur
  gating, no event Set.

Each is a true singleton (the constructor returns the existing `_instance`), so a re-render reuses
the one live loop instead of stacking intervals.

## AnswerTracker — submit and "Try Again"

`AnswerTracker` (a singleton, `web/src/problem.js`) dedupes submissions and infers a wrong answer
without the server round-tripping a verdict:

- `reportAnswer` ignores an empty answer or a resubmit of the identical string, removes the
  `working_on_problem` ticker, and POSTs `answered_problem`. It returns whether it actually fired
  (the return drives the `submitting` flag).
- `wasIncorrectAnswer` is true when a non-empty answer was submitted, the input has not changed
  since, and the problem id is unchanged — i.e. we submitted, the server did NOT advance us to a
  new problem, and the kid hasn't started retyping. That renders the "Try Again!" alert. A correct
  answer advances `problem_id` via the response, so the alert never shows for it.
- `problemWasDisplayed` resets tracker state when a new problem id appears.

The wrong-answer signal is thus **inferred from non-advancement**, not from an explicit verdict
field — see Gotchas.

## Report-problem flow

A kid-visible "Report problem" link opens a PIN-gated modal. `handleReportSubmit` (`web/src/play.js`)
checks the 4-digit PIN client-side against `user.pin` (and requires a PIN to already be set in
settings), then posts `bad_problem_user` with `{problem_id, explanation}` — explanation capped at
`REPORT_EXPLANATION_MAX_LENGTH`. If the response carries a fresh gamestate, the problem is swapped
out.

## Video playback

`VideoView` (`web/src/video.js`) wraps `react-player`. The reward video is the gate-clear: it shows
once `solved >= target`. Notable behavior:

- The `list` query param is stripped from the URL so a single video plays instead of an embedded
  playlist.
- Spacebar toggles play/pause via a global `document.body.onkeyup` handler; a transparent
  `#click-blocker` overlay intercepts clicks to the same toggle so the kid can't reach YouTube's
  own chrome.
- `onProgress` reports the **delta** since the last tick, not cumulative elapsed, so the server can
  sum watch-time correctly.

`VideoCompanionView` (`web/src/video_companion.js`) is the read-only mirror: no events, no keyboard
handler, click-to-play only.

## Attempt reconstruction

`getEvents` (`web/src/companion.js`) walks the polled events newest-first and rebuilds the attempts
for the *current* problem only: it buffers `answered_problem` events and, at each
`selected_problem` boundary, stops once it reaches a selection for a different `problem_id`,
otherwise flushing the buffer into `attempts`. The result is rendered with relative timestamps by
`AttemptTime` (`web/src/problem_companion.js`).

## Invariants

- **The loop branch is `solved >= target` on both surfaces.** Change one, change both (`PlayView`,
  `CompanionView`).
- **Event-type strings must match `server/api/enums.go`.** They are bare literals on the client; a
  typo silently drops the event (#279).
- **`PreprocessExpression` is shared** across play, problem, and companion so the kid view and the
  adult mirror can never disagree about how an expression looks.
- **The singletons must stay singletons.** `EventReporterSingleton`, `AnswerTracker`, and
  `RefresherSingleton` each guard `_instance`; dropping the guard stacks duplicate
  intervals/handlers on every re-render.
- **Companion is read-only.** GET-only REST endpoints, no events; the answer is shown only here,
  never on `/play`.

## Gotchas / non-obvious behavior

- **"Try Again!" is inferred, not told.** The client decides an answer was wrong purely from the
  server NOT advancing `problem_id` in the `answered_problem` response (`AnswerTracker`). If the
  server ever returned the same problem after a correct answer, the kid would wrongly see
  "Try Again!".
- **`debug_quickplay` auto-plays the loop.** When `conf.debug_quickplay` is true, `PlayView`
  auto-posts `working_on_problem` then the correct `problem.answer`, and auto-watches the video,
  reloading `/play` each step — a dev fast-forward, shipped `false` in `conf.json`.
- **`video.url` is mutated in place.** `VideoView` rewrites `video.url` on the prop object rather
  than copying — harmless because the view re-fetches, but it mutates a prop.
- **Global keyup handler is reassigned, not added.** `VideoView` sets `document.body.onkeyup`
  directly, overwriting any prior handler each render; it is not an `addEventListener` and does not
  clean up. Fine only because the video view is the sole setter.
- **Render-phase side effects.** Both views construct singletons and call `postEvent` /
  `eventReporter.add` during render rather than in an effect (`PlayView`, `ProblemView`). It works
  only because the singletons are idempotent; it is not idiomatic React and re-runs on every render.

## Related files

- `web/src/index.js` — `genPostEventFcn` (the `/events` POST), `MainView` route table,
  `conf.event_reporting_interval` wiring.
- `web/src/conf.json` — `event_reporting_interval`, `debug_quickplay`.
- `web/src/problem_companion.js`, `web/src/video_companion.js` — read-only mirror sub-views.
- `web/src/pin.js` — `RequirePin`, `ClearSessionPin` (companion gate / play PIN clear).
- `server/api/enums.go` — authoritative event-type constants.
- `server/api/meta_models.go` — `PlayData`, the `/play` response shape.
- `server/api/custom_handlers.go` — `customGetPlayData` (the `/play` handler, video-count gate,
  problem reselection).
- `server/api/process_events.go` — server-side event handling (separate area).

## Extension checklist — adding a client event

1. Add the constant to `server/api/enums.go` (and its server handler / record-only entry).
2. Emit it from the relevant view via `eventReporter.postEvent` / `postEvent` with a stringified
   `value`.
3. If it should advance the loop, have the server return a fresh `{ gamestate, problem, video }`
   and swap it in like `answered_problem` (`PlayView`).
4. If it should accrue over time, add it to the `EventReporterSingleton` Set (sticky) and `remove`
   it at the right boundary.
5. Add a row to the event table above and cite the emit site by symbol.
