# Accounts: identity, roles, and the parent PIN

How a person becomes a `users` row, what they're allowed to do (the authorization role), and the
two client-side gates — Auth0 login and the four-digit parent PIN — that wrap the kid-facing app.
**Change this doc in the same PR as any behavior change here.** This area is prose (no doc-sync
anchors); `make docs-check BASE=origin/master` flags a PR that touches the owned files without
touching this doc.

Owned files: `server/api/roles.go`, `web/src/auth0.js`, `web/src/pin.js`, `web/src/setup.js`.

## The model

Three independent layers, in order of authority:

| Layer | Source of truth | Purpose | Client-bypassable? |
|---|---|---|---|
| **Identity** (Auth0) | Auth0-issued JWT, `sub` claim | proves *who* the caller is | no — JWT validated server-side (`auth0.EnsureValidToken` in `init.go`) |
| **Authorization** (role) | `users.role` column | gates operator-only surfaces | no — server `RequireAdmin` |
| **Parent PIN** | `users.pin` column | keeps a *kid* out of adult settings | yes — client-side gate only (see Gotchas) |

The Auth0 `sub` is the `auth0_id`; every server handler resolves it to a `users` row via
`UserMiddleware` (`server/common/middleware.go`) before doing anything else. One Auth0 account =
one `users` row = one family/operator; there is no per-kid login (kids are distinguished by the PIN
gate and the companion view, not by identity).

## Roles

Two roles, string constants in `roles.go` (`RoleStudent`, `RoleAdmin`):

| Constant | Value | Who | How assigned |
|---|---|---|---|
| `RoleStudent` | `"student"` | every account by default | column default in `migrations/41.sql` |
| `RoleAdmin` | `"admin"` | the single operator | manual `UPDATE users SET role='admin' WHERE auth0_id='...'` |

There is **no seeding machinery and no self-service promotion** — the operator is promoted by hand
in the DB. The role lives only on the `users` row; the DB is the single source of truth.

**Server gate — `RequireAdmin`** (`roles.go`): a gin middleware that aborts with 403 unless the
loaded user's role is `RoleAdmin`. It reads the user that `UserMiddleware` loaded, so it must be
registered after it. All operator surfaces live under the `/api/v1/admin` group, which composes
`userMiddleware` then `RequireAdmin()` (`init.go`). Inhabitants: `GET /admin/whoami`
(`adminWhoami`, a liveness/first-inhabitant echo of the caller's auth0_id/id/role) and the
difficulty-calibration endpoints (owned by another area).

**Client gate — admin UI** (`web/src/index.js`, `isAdmin`): role-derived flag that drives three
things — the "Admin" nav button renders only for an admin; admin routes (`/admin`,
`/admin/difficulty-calibration`, `/admin/style-guide`) render their view for an admin and the
**404 page** for everyone else, so a non-admin gets no hint the surface exists; and admin paths
**bypass the setup-wizard gate** (the `onAdminPath` short-circuit), so an operator reaches admin
tools without completing kid onboarding. The client gate is cosmetic; server `RequireAdmin` is
authoritative — a forged request to `/api/v1/admin/*` still gets 403. Note `/admin/style-guide`
is gated only client-side (no server `/admin` data endpoint backs it).

## Identity / Auth0 (`web/src/auth0.js`)

Three buttons wrapping `@auth0/auth0-react`: `LoginButton` and `SignupButton` both
`loginWithRedirect` (differing only in styling), `LogoutButton` calls `logout` returning to the
window origin. The `Auth0Provider` is configured once at the app root (`index.js`,
`cacheLocation: "localstorage"`); after login the app pulls an access token with
`getAccessTokenSilently` and sends it as a `Bearer` token on every API call.

**First-login provisioning** (`index.js`): on page load the app GETs `/pageload/:auth0_id`; a 404
means no `users` row yet, so it POSTs `/users` with `{auth0_id, email, username}` from the Auth0
profile, then re-fetches. The create path (`customCreateOrUpdateUser`) inserts only
`auth0_id, email, username` (`createUserSQL`), so a new row always takes the DB defaults
`role='student'` and `pin=''` regardless of request body. The empty PIN is what triggers the setup
wizard.

## The parent PIN (`web/src/pin.js`, `web/src/setup.js`)

A **four-digit code** stored in `users.pin`, cached in `sessionStorage` under `math-game-pin`. It
keeps a kid from wandering into adult surfaces.

`pin.js` exports:

| Export | Contract |
|---|---|
| `SetSessionPin` / `GetSessionPin` / `ClearSessionPin` | read/write/clear the `sessionStorage` entry |
| `RequirePin(correctPin)` | route guard: redirects to `/pin/<encoded current path>` unless the session PIN is considered valid; returns whether access is allowed (see Gotchas — its check is buggy) |
| `PinView` | four-digit entry component (`react-pin-input`), used in setup (`isSetup`) and at the `/pin/:redirect_pathname` gate route |

Two-stage gate for a protected surface: a guarded view (`/settings`, `/companion/:student_id`)
calls `RequirePin`, which on a missing/invalid session PIN redirects to the `/pin/...` route; that
route renders `PinView` in gate mode, which validates length ≥ 4 and `pin === user.pin`, stores the
session PIN, and redirects back to the originally requested path. `PinView`'s gate-mode check is
the correct one; `RequirePin`'s is not (Gotchas).

### Setup wizard (`setup.js`)

`SetupView` is a four-step tabbed flow (`allTabs`), shown by the main view whenever a logged-in
account is off an admin path and has `user.pin === ""` **or** `numEnabledVideos < 3`:

1. **Problem Types** — continue gated on a valid bitmap (`problem_type_bitmap >= 1`).
2. **Add Videos** — playlists + videos; continue gated on ≥ 1 enabled video.
3. **Set Parent Pin** — `PinView` in `isSetup` mode; continue POSTs the user with the freshly-set
   session PIN (`PinTabView`).
4. **Start Playing!** — requires ≥ 1 enabled playlist, then links to `/play`.

Tabs advance forward only; you may click *back* to an already-visited tab but not skip ahead
(`handleTabClick`). The PIN reaches the server via POST `/users/:auth0_id` (`customUpdateUser`),
which lets a caller change their own `email`/`username`/`pin` but force-overwrites `role` and `id`
from the stored row — so this endpoint can never self-promote to admin even though the bound `User`
struct includes a `role` field.

## Invariants

- **Role is never client-settable.** Neither create (`createUserSQL` omits `role`) nor update
  (`customUpdateUser` forces `model.Role` from the stored row) takes the role from request input.
  Promotion is DB-only.
- **A caller may only mutate their own row.** `customUpdateUser` returns 403 when the URL
  `auth0_id` isn't the authenticated identity.
- **`RequireAdmin` runs after `UserMiddleware`.** It depends on the loaded user; registering it
  earlier always 403s.
- **Admin surfaces are double-gated.** Server `RequireAdmin` (authoritative) + client `isAdmin`
  route guard (renders 404 to non-admins).
- **New rows default to `student` / empty PIN.** The empty PIN is the signal that drives a new
  account into the setup wizard.

## Gotchas / non-obvious behavior

- **The PIN is a UX gate, not a security boundary.** It is checked entirely in the browser against
  `user.pin`, which the client already holds (returned in the pageload payload). It exists to stop
  a *kid* from tapping into settings, not to authorize anything. All real authorization is the
  Auth0 JWT + role.
- **`RequirePin` is buggy and it guards real surfaces.** `RequirePin` deems a present session PIN
  valid when it does *not* equal the argument (the inverted comparison), and both its callers pass
  `user.id` rather than a PIN (`settings.js` and `companion.js`, `RequirePin(user.id)`). Net
  effect: any 4-digit session PIN that isn't the stringified user id passes the guard on
  `/settings` and `/companion/:student_id` (#274). The earlier doc claimed `PinView` guards
  `/settings`; in fact `RequirePin` is the first-stage guard there, and the correct `PinView`
  gate-mode check only runs if `RequirePin` actually redirects to `/pin/...`.
- **`ClearSessionPin` fires on several routes.** Rendering the 404 page, the home view, or the
  play view clears the session PIN (`index.js`, `home.js`, `play.js`), so leaving a protected area
  drops the gate.
- **Two different enabled-video thresholds.** The setup gate re-shows when `numEnabledVideos < 3`
  even for an already-set-up account, while the wizard's final step only requires ≥ 1 enabled
  playlist/video — 3 to *exit* the gate, 1 to *finish* the wizard.

## Related files

- `server/api/roles.go` — `RoleStudent`, `RoleAdmin`, `RequireAdmin`, `adminWhoami`.
- `server/api/init.go` — Auth0 JWT + user-middleware wiring (`EnsureValidToken`,
  `Auth0IdMiddleware`, `UserMiddleware`); the `/admin` group composition.
- `server/common/middleware.go` — `Auth0IdMiddleware`, `TestAuth0IdMiddleware`, `UserMiddleware`.
- `server/api/handler_helpers.go` — context accessors (`GetAuth0IdFromContext`,
  `GetUserFromContext`/`Lenient`).
- `server/api/custom_handlers.go` — `customUpdateUser`, `customCreateOrUpdateUser`.
- `server/api/migrations/41.sql` — adds `users.role` (default `student`).
- `server/api/models.json` (`users` table) — `pin` and `role` fields; regenerate
  `user_model.generated.go` (which holds `createUserSQL`) via `make build-api`, never edit it.
- `web/src/index.js` — Auth0 provisioning, admin route guards, the setup gate.
- `web/src/auth0.js`, `web/src/pin.js`, `web/src/setup.js` — owned files.
