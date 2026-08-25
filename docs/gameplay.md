# The gameplay loop (frontend)

The kid-facing play surface: how a problem is fetched, rendered, answered, and how a reward video
plays, plus the client-side event reporting that drives the server's adaptive loop.
**Change this doc in the same PR as any behavior change here**;
`make docs-check BASE=origin/master` fails when the owned files (`web/src/play.js`,
`web/src/problem.js`, `web/src/video.js`) change without this doc.

This area is `type=prose` — it owns React view code, not pinned constants, so there is no doc-sync
anchor block. The doc stops at the HTTP boundary: what the client sends and what it expects back.
The server-side counterpart (event processing, gamestate mutation, problem selection) lives in
`server/api` and is covered by `docs/problem-generation.md` and the event-processing area.

## The model

The loop renders from a **gamestate** — the server's per-user cursor
(`Gamestate`, `server/api/gamestate_model.generated.go`): `{ user_id, problem_id, video_id, solved,
target }`. Its central branch is:

```
gamestate.solved >= gamestate.target  ?  show the reward video  :  show the problem
```

`solved` / `target` also drive the progress-meter width (`ProblemView`) and, one problem out from
the reward (`target - solved === 1`), the meter's `final` modifier: the bar switches from
`$color-one-contrast` to `$color-reward` and `ProblemView` shows a
"1 more until your video" cue above it.

One route renders it: `/play` → `PlayView` (`web/src/play.js`), mounted by `MainView` in
`web/src/index.js`. It is the kid's surface, and the only one that mutates gameplay state.

### PlayView data flow (`/play`)

1. **Fetch.** On mount, GET `/play/:user.id` returns `{ gamestate, problem, video }` (`PlayView`;
   server shape `PlayData`, `server/api/meta_models.go`). A **403 calls `refreshPageLoadData`** —
   `customGetPlayData` returns Forbidden below the playable-video floor (`minPlayableVideos`, the
   same floor the client gates on; see [accounts.md](accounts.md)), and the fresh count is what
   lets `useTakeover` put the videos repair page on the screen — the person at a 403 is signed in
   and a playlist away from playing, and that page is where they fix it. **If nothing takes the
   screen, this view says why**: still being mounted after that refresh means the read failed, or
   the 403 was not the video floor at all (`RequireSelf` raises one too), so `PlayView` renders the
   stock message block (`.not-found`, see `/style-guide`) pointing at Settings rather than a
   spinner that never resolves.
   Empty / invalid bodies are logged and swallowed. **A response landing after unmount is
   dropped** (the effect's `cancelled` flag): that refresh rewrites app-wide state, and whatever
   replaced this view must not read a payload its predecessor's answer overwrote. Pinned by
   `play.test.js`.
2. **Render LaTeX.** `problem.expression` is run through `PreprocessExpression` and rendered to an
   HTML string with KaTeX (`PlayView`, the `renderLatex` effect). `PreprocessExpression` wraps
   multi-digit numbers in `\text{}`, splits `\text{}` blocks for word-wrap, and escapes a bare `%`
   to `\%` (KaTeX reads a bare `%` as a comment and would eat the rest of the expression). A render
   failure posts a
   `bad_problem_system` event and swaps in the server-supplied replacement problem; if the server
   returns no replacement, it reloads `/play`. A corrupt expression self-heals without a visible
   error.
3. **Answer.** `ProblemView` holds the answer input; submitting (Enter or the button) posts
   `answered_problem` with the typed string.
4. **Advance.** The `answered_problem` response carries a fresh `{ gamestate, problem, video }`,
   which `PlayView` swaps in (the `eventReporter` callback, on the `answered_problem` branch),
   re-rendering the next problem — or the video, once `solved >= target`.

## Event types reported from the client

Every event is POSTed to `/events` as `{ event_type, value }` with `value` stringified
(`genPostEventFcn`, `web/src/index.js`). Event-type strings come from `EventTypes` in
`web/src/enums.generated.js`, generated from the server constants in `server/api/event_types.go` by
`make build-api`. `TestEventTypesMatchJS` pins the two sets to each other, and rejects both a known
event type spelled as a literal anywhere else under `web/src` and any literal handed to an
event-posting call site — the second catch is what makes a *misspelled* type fail CI, since it
matches no constant to be recognized by. The table is the subset this area emits.

| Event | Value | Emitted by | When |
|---|---|---|---|
| `working_on_problem` | interval ms | `EventReporterSingleton` ticker | every interval while focused and a problem is shown |
| `answered_problem` | typed answer string | `AnswerTracker.reportAnswer` | submit |
| `watching_video` | elapsed delta ms | `VideoView` `onTimeUpdate` | during playback, at most once per interval |
| `done_watching_video` | video id | `VideoView` `onEnded` | video finishes |
| `error_playing_video` | error | `VideoView` `onError` | playback error |
| `bad_problem_system` | `{problem_id, explanation}` JSON | `PlayView` `renderLatex` catch | KaTeX throws |
| `bad_problem_user` | `{problem_id, explanation}` JSON | `PlayView` `handleReportSubmit` | adult reports a bad problem |

`done_watching_video` and `error_playing_video` both navigate back to `/play`, forcing a full
reload and a fresh gamestate fetch. The server also defines `logged_in`, `selected_problem`,
`solved_problem`, and the `set_*` settings events — none are emitted from this area.

## The reporting singleton

**`EventReporterSingleton`** (`web/src/play.js`) holds a `Set` of "sticky" event types re-POSTed
every `interval` ms (`conf.event_reporting_interval`). `working_on_problem` is the only sticky
member: `ProblemView` adds it while a problem is shown and `AnswerTracker` removes it on submit, so
"time on problem" accrues only while the kid is actually looking at one. The ticker is focus-gated —
a no-op while the window is blurred — which keeps traffic off backgrounded tabs.

It is a true singleton (the constructor returns the existing `_instance`), so a re-render reuses
the one live loop instead of stacking intervals. Re-construction **hands over** the caller's
callback (`postEvent` / `eventReporter`) to the existing instance: a remount otherwise leaves the
singleton posting through the unmounted view's closure.

`PlayView` builds the reporter in an effect and tears it down on unmount — the constructor attaches
focus/blur listeners and starts the interval, so it must not run during render, and those handlers
are bound once and stored so `removeEventListener` can actually match them. Because `postEvent` is
a fresh function on every parent render (`index.js` calls `genPostEventFcn()`), the reporter reads
it through a ref rather than being rebuilt, which would re-arm the interval constantly.

`ProblemView` owns the sticky membership through one effect: it adds `working_on_problem` while an
unanswered problem is mounted and removes it on cleanup, which covers submitting, advancing to the
next problem, and unmounting to the reward video. `src/problem_reporting.test.js` pins that
lifecycle.

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

"Try Again!" renders into `.problem-feedback`, a slot whose height is reserved whether or not the
nudge is showing, so a wrong answer never moves the answer box or anything under it. It is styled
in `$color-ink-soft` and never red — red is reserved for parent/admin validation (see the style
guide's colour invariant).

## Report-problem flow

A kid-visible "Skip problem" link opens the shared `PinConfirmModal`
(`web/src/pin_confirm_modal.js`; the modal shape is documented on `/style-guide`), which holds the
typed PIN and won't enable Submit until all four digits are in. This page contributes only the
optional explanation field, passed as children and styled by `.report-explanation` in `play.scss`.

`handleReportSubmit` (`web/src/play.js`) receives the entered PIN, checks it client-side against
`user.pin` (and requires a PIN to already be set in settings), then posts `bad_problem_user` with
`{problem_id, explanation}` — explanation capped at `REPORT_EXPLANATION_MAX_LENGTH`. If the response
carries a fresh gamestate, the problem is swapped out.

## Video playback

`VideoView` (`web/src/video.js`) wraps `react-player`. The reward video is the gate-clear: it shows
once `solved >= target`. Notable behavior:

- **Rotation is a preference, not a rule.** `selectVideo` (`server/api/custom_handlers.go`) is
  called with the just-watched video excluded, so the reward does not repeat back-to-back when
  the pool can spare another video — and the exclusion **yields when it can't**, handing the same
  video back rather than the `nullVideoId` sentinel. That is what makes a one-video pool a
  supported setup rather than a per-cycle error: see the floor in [accounts.md](accounts.md).
  Yielding cannot resurrect a broken video, because `error_playing_video` disables it and
  disabled videos never reach the candidate list.

- The `list` query param is stripped from the URL so a single video plays instead of an embedded
  playlist.
- **The embed is served from `youtube-nocookie.com`**, YouTube's privacy-enhanced host. `react-player`
  picks the embed domain from the host in `src`, so `VideoView` rewrites the stored
  `youtube.com/watch` URL onto the no-cookie host before handing it over. The JS API, events and
  playback are the same on either host. What this buys is that the view is kept out of ad
  personalization. It does **not** buy a cookie-free screen: the host defers cookies until
  playback, and playback is the whole point here, so the privacy copy's "when a video plays,
  YouTube may set cookies" stays accurate as written.
- Spacebar toggles play/pause via a global `document.body.onkeyup` handler; a transparent
  `#click-blocker` overlay intercepts clicks to the same toggle so the kid can't reach YouTube's
  own chrome.
- `onTimeUpdate` reports the **delta** since the last report, not cumulative elapsed, so the server
  can sum watch-time correctly. The player fires it on its own polling cadence, so `VideoView`
  throttles the reports down to the caller's `interval`.

## Invariants

- **The loop branch is `solved >= target`** (`PlayView`).
- **Event types are referenced through `EventTypes`, never as literals.** `TestEventTypesMatchJS`
  enforces it; a literal that slipped past would be free to drift from `server/api/event_types.go`,
  and the server then just doesn't recognize what it receives.
- **The singletons must stay singletons.** `EventReporterSingleton` and `AnswerTracker` each guard
  `_instance`; dropping the guard stacks duplicate intervals/handlers on every re-render.
- **The answer is never rendered on `/play`.** `problem.answer` reaches the client (the
  `debug_quickplay` fast-forward reads it), but no kid-facing view displays it.

## Gotchas / non-obvious behavior

- **"Try Again!" is inferred, not told.** The client decides an answer was wrong purely from the
  server NOT advancing `problem_id` in the `answered_problem` response (`AnswerTracker`). If the
  server ever returned the same problem after a correct answer, the kid would wrongly see
  "Try Again!".
- **`debug_quickplay` auto-plays the loop.** When `conf.debug_quickplay` is true, `PlayView`
  auto-posts `working_on_problem` then the correct `problem.answer`, and auto-watches the video,
  reloading `/play` each step — a dev fast-forward, shipped `false` in `conf.json`. It runs from an
  effect (it posts events), and both views render `null` while it drives.
- **The wrong-answer check reads tracker state before the reset effect runs.** `problemWasDisplayed`
  now resets in an effect, so on the first render of a *new* problem the tracker still holds the
  previous answer. `wasIncorrectAnswer` is unaffected because it also requires
  `lastProblemId === problem_id`, which is false on exactly that render; the reset lands before any
  render where the ids match. Removing that id comparison would resurrect a stale "Try Again!".

## Related files

- `web/src/index.js` — `genPostEventFcn` (the `/events` POST), `MainView` route table,
  `conf.event_reporting_interval` wiring.
- `web/src/api.js` — `apiFetch`, which issues the `/play` GET and the `/events` POST (and every
  other authenticated call). Returns the raw `Response`, so the 403 handling above is `PlayView`'s
  own. See [accounts.md](accounts.md).
- `web/src/conf.json` — `event_reporting_interval`, `debug_quickplay`. Generated and gitignored,
  so the suites that reach it resolve `web/src/conf.test.json` instead (the `test.alias` in
  `web/vite.config.js`).
- `web/src/problem_reporting.test.js` — pins the `working_on_problem` add/remove lifecycle.
- `web/src/pin.js` — `RequirePin`. The play view no longer clears the session PIN itself; one
  pathname-keyed rule in `index.js` owns that (see [accounts.md](accounts.md)).
- `web/src/pin_confirm_modal.js` — the shared PIN-confirmation modal the report flow renders;
  shape and styles are the design system's (`/style-guide`, `components.scss`).
- `web/src/enums.generated.js` — `EventTypes` / `ProblemTypes`, generated by `make build-api`;
  never hand-edit.
- `server/api/event_types.go` — authoritative event-type constants.
- `server/api/event_types_js_sync_test.go` — `TestEventTypesMatchJS`, the Go↔JS pin.
- `server/api/meta_models.go` — `PlayData`, the `/play` response shape.
- `server/api/custom_handlers.go` — `customGetPlayData` (the `/play` handler, video-count gate,
  problem reselection).
- `server/api/process_events.go` — server-side event handling (separate area).

## Extension checklist — adding a client event

1. Add the constant to `server/api/event_types.go` (and its server handler / record-only entry),
   then `make build-api` to regenerate `web/src/enums.generated.js`.
2. Emit it from the relevant view via `eventReporter.postEvent` / `postEvent` with a stringified
   `value`, referencing it as `EventTypes.<NAME>`.
3. If it should advance the loop, have the server return a fresh `{ gamestate, problem, video }`
   and swap it in like `answered_problem` (`PlayView`).
4. If it should accrue over time, add it to the `EventReporterSingleton` Set (sticky) and `remove`
   it at the right boundary.
5. Add a row to the event table above and cite the emit site by symbol.
