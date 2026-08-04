# Accounts: identity, roles, and the parent PIN

How a person becomes a `users` row, what they're allowed to do (the authorization role), and the
two client-side gates — Auth0 login and the four-digit parent PIN — that wrap the kid-facing app.
**Change this doc in the same PR as any behavior change here.** This area is prose (no doc-sync
anchors); `make docs-check BASE=origin/master` flags a PR that touches the owned files without
touching this doc.

Owned files: `server/api/roles.go`, `server/api/self_access.go`, `web/src/auth0.js`, `web/src/pin.js`,
`web/src/setup.js`.

## The model

Four independent layers, in order of authority:

| Layer | Source of truth | Purpose | Client-bypassable? |
|---|---|---|---|
| **Identity** (Auth0) | Auth0-issued JWT, `sub` claim | proves *who* the caller is | no — JWT validated server-side (`auth0.EnsureValidToken` in `init.go`) |
| **Ownership** (self-only) | the loaded `users` row vs. the path id | keeps one account out of another's data | no — server `RequireSelf` |
| **Authorization** (role) | `users.role` column | gates operator-only surfaces | no — server `RequireAdmin` |
| **Parent PIN** | `users.pin` column | keeps a *kid* out of adult settings | yes — client-side gate only (see Gotchas) |

The Auth0 `sub` is the `auth0_id`; every server handler resolves it to a `users` row via
`UserMiddleware` (`server/common/middleware.go`) before doing anything else. One Auth0 account =
one `users` row = one family/operator; there is no per-kid login (a kid is kept out of adult
surfaces by the PIN gate, not by identity).

## Self-only access (`server/api/self_access.go`)

Naming a user in a request path entitles you to nothing. `RequireSelf` is a gin middleware that
compares every user-naming path param against the caller's own identity and 403s on a mismatch:

| Path param | Compared against |
|---|---|
| `:auth0_id` | the validated token's `sub` (`GetAuth0IdFromContext`) |
| `:user_id` | `users.id` of the row `UserMiddleware` loaded from that `sub` |

It reads the loaded user, so like `RequireAdmin` it must be registered after `UserMiddleware`. A
route carrying neither param passes straight through, which is what makes it safe to register on the
whole authenticated surface instead of route by route — and registering it on the group rather than
per handler is the whole point: **a new route is self-only by construction, not by remembering.**
`init.go` puts every authenticated route inside one
`authed := v1.Group("", userMiddleware, a.RequireSelf())`. Exactly two routes sit outside it, and
neither names a user:

| Outside `authed` | Why |
|---|---|
| `POST /users`, `POST /users/` | a first-login caller has no `users` row to load yet, so this takes the *lenient* middleware; `customCreateOrUpdateUser` takes the `auth0_id` from the token |
| `GET /problems/:id` | a problem row belongs to nobody, so no `users` row is needed (the validated JWT still is) |

Because the guard runs ahead of every handler, **handlers do not re-check — and every handler that
writes sources the row it writes from the token-loaded identity**, never from the path or body
(`customUpdateSettings` and `customUpdateUser` overwrite the bound model's key from context;
`customDeleteAccount` and `getStatistics` never bind one). So even a handler mistakenly registered
outside `authed` could only ever touch the caller's own data. The path id's only job is to be
checked by `RequireSelf`. That single enforcement point is deliberate: the per-handler version was
three forgettable lines, three handlers remembered it, and six didn't — which is the bug this
closes (#331). The `Test*Write_IgnoresClientSupplied*` cases pin the token-sourcing half.

`TestSelfOnly_*` (`self_access_test.go`) is what keeps it true. It enumerates `router.Routes()` for
the params listed in `selfOnlyPathParams` instead of a hand-maintained list, then asserts every such
route 403s when the caller asks for another user's id — so a newly registered user-scoped route is
covered the moment it exists, and one registered *outside* `authed` fails CI. A companion case
asserts own-id requests still return 200, so a guard that 403'd everything couldn't satisfy it. And
because all of that keys off `selfOnlyPathParams`, `TestSelfOnly_NoUnknownPathParams` closes the
remaining gap: a route param in neither `selfOnlyPathParams` nor the known non-user list fails CI,
so a user-naming param under a new name (`:student_id`, `:target_id`) can't bypass the guard
unclassified.

**One of the holes was a write, not a read.** `POST /settings/:user_id` binds `user_id` from the URI
and hands it to `UPDATE settings ... WHERE user_id=?`, so before the guard any signed-in account
could reshape another account's problem-type bitmap, target difficulty and work percentage.
`TestSelfOnly_CannotWriteAnotherUsersSettings` pins that the victim's row is left untouched.

### The PIN is still sent to the client, deliberately

`GET /users/:auth0_id` and `GET /pageload/:auth0_id` still serialize `users.pin`, and the client
gate still compares against `user.pin`. Self-only enforcement means that payload now only ever
reaches a token for that same account — and such a token can already overwrite the PIN outright via
`customUpdateUser`. Removing `pin` from the payload alone would therefore buy nothing: it would push
`RequirePin` into a server-side verify call while leaving the PIN exactly as writable. Hiding the
PIN only becomes worthwhile together with real re-authentication (an Auth0 `max_age`/`prompt=login`
round trip), which is the same prerequisite the deletion section below already records.

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

## Auth0 tenant objects

Three distinct objects in the Auth0 dashboard, one config key each. They are easy to conflate
because two of them are named after the product and a third is named after the domain:

| Auth0 object | Type | Config key | Breaks if wrong |
|---|---|---|---|
| the game's front end | Single Page Application | `auth0_clientId` | login redirect fails; this ID is public and ships in the JS bundle |
| the game's API | Custom API (resource server) | `auth0_audience` | every authenticated request 401s: the SPA mints tokens *for* this identifier and `EnsureValidToken` validates against it |
| the deletion client | Machine to Machine | `auth0_management_clientId` / `Secret` | account deletion still scrubs our DB, but the Auth0 identity is orphaned |

The M2M application must be authorized against the **Auth0 Management API** (built in to every
tenant, identifier `https://<canonical-domain>/api/v2/`) with the `delete:users` scope — *not*
against the game's own Custom API, which would hand the server a token to call itself with.

**Two domains, and they are not interchangeable.** `auth0_domain` is whatever domain issues logins,
because it is the JWT issuer that `EnsureValidToken` builds its JWKS URL and issuer check from. When
that is a **custom** domain, Management API calls cannot use it: Auth0 answers
`403 access_denied, "Service not enabled within domain"` for the `/api/v2/` audience, which is only
served on the tenant's canonical `<tenant>.<region>.auth0.com`. That is what `auth0_management_domain`
is for. Leave it empty on a tenant with no custom domain and it falls back to `auth0_domain`
(`managementDomain()`, pinned by `TestManagementDomain_FallsBackToIssuerDomain`).

## Identity / Auth0 (`web/src/auth0.js`)

Two buttons wrapping `@auth0/auth0-react`: `LoginButton` calls `loginWithRedirect`, `LogoutButton`
calls `logout` returning to the window origin. (A third, `SignupButton`, was removed with the old
React landing page — it was byte-identical to `LoginButton` apart from styling.) The
`Auth0Provider` is configured once at the app root (`index.js`,
`cacheLocation: "localstorage"`); after login the app pulls an access token with
`getAccessTokenSilently` and sends it as a `Bearer` token on every API call.

**Entry point.** The marketing page at `/` is static HTML outside the React app, so it cannot call
Auth0 itself; its CTAs link to `/login`, a route whose only job is to fire `loginWithRedirect` (and
to bounce an already-signed-in visitor to `/play`). Auth0 then redirects back to the registered
callback, the site origin — which is that same static page, with no SDK to finish the exchange. A
few lines at the top of the landing hand `?code`/`&state` (and `?error`) to `/login` so the app
completes the callback. That indirection is deliberate: it keeps the registered callback URL
unchanged, so nothing in the Auth0 dashboard has to move. See #329.

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
| `RequirePin(correctPin)` | route guard: redirects to `/pin/<encoded current path>` unless the session PIN equals `correctPin`; returns whether access is allowed |
| `PinView` | four-digit entry component (`react-pin-input`), used in setup (`isSetup`) and at the `/pin/:redirect_pathname` gate route |

Two-stage gate for a protected surface: a guarded view (`/settings`) calls `RequirePin`, which on a
missing/invalid session PIN redirects to the `/pin/...` route; that route renders `PinView` in gate
mode, which validates length ≥ 4 and `pin === user.pin`, stores the session PIN, and redirects back
to the originally requested path. Both checks compare the entered
PIN against `user.pin` by equality.

### Setup wizard (`setup.js`)

`SetupView` is a four-step tabbed flow (`allTabs`), shown by the main view whenever a logged-in
account is off an admin path and has `user.pin === ""` **or** `numEnabledVideos < 3`:

1. **Problem Types** — continue gated on a valid bitmap (`problem_type_bitmap >= 1`).
2. **Add Videos** — playlists (each expandable to its videos); continue gated on
   `MIN_PLAYABLE_VIDEOS` playable videos. The count is reported up by
   `PlaylistsSettingsView`'s `onPlayableCountChange` rather than derived from a second
   list — see [settings.md](settings.md).
3. **Set Parent Pin** — `PinView` in `isSetup` mode; continue POSTs the user with the freshly-set
   session PIN (`PinTabView`).
4. **Start Playing!** — requires ≥ 1 enabled playlist, then links to `/play`.

Tabs advance forward only; you may click *back* to an already-visited tab but not skip ahead
(`handleTabClick`). The PIN reaches the server via POST `/users/:auth0_id` (`customUpdateUser`),
which lets a caller change their own `email`/`username`/`pin` but force-overwrites `role` and `id`
from the stored row — so this endpoint can never self-promote to admin even though the bound `User`
struct includes a `role` field.

## Account deletion (`server/api/delete_account.go`, `DeleteAccountView` in `settings.js`)

`DELETE /api/v1/users/:auth0_id` is the only self-service way out. It is immediate and
irreversible: there is no recovery window and no soft-delete flag.

**Two server-side checks.** The path `auth0_id` must equal the token-authenticated identity (you can
only delete yourself — `RequireSelf` on the route, per the self-only section above), and the request
body must carry the account's PIN, compared against
`users.pin` on the server. An account with no PIN set is **not** deletable (`pin` defaults to
`''`, so an empty submitted PIN would otherwise satisfy an equality check); the caller is told to
set one first.

**The PIN check here confirms intent; it is not a second factor.** Being server-side makes it
harder to skip than the browser-side `RequirePin` gate, but it adds no security against a stolen
access token: the same token reads the PIN back from `GET /pageload/:auth0_id` and can overwrite it
outright via `customUpdateUser`. Per the PIN section above, the PIN is a kid gate, and the
authorization boundary for deletion is the JWT plus the self-only check. Making deletion resistant
to a compromised token needs a real re-authentication (an Auth0 `max_age`/`prompt=login` round trip
before the DELETE), which this endpoint does not do.

**Anonymize in place, don't delete the row.** `anonymizeAndPurgeUser` runs one transaction that
hard-deletes every purely per-user table (`perUserDeleteSQL`) and then scrubs the `users` row
rather than removing it:

| Column | After deletion |
|---|---|
| `auth0_id` | `deleted-<id>` sentinel — breaks the login link, stays unique per user |
| `email`, `username`, `pin` | `''` |
| `id`, `role` | unchanged |

`events` is the one user-keyed table deliberately **retained** (`retainedUserTables`): its
aggregate solve-time data feeds difficulty calibration. The `users` row survives to hold that
grouping key steady. There is no DB-level foreign key on `events.user_id` (see
`CreateEventTableSQL`) — the reason to keep the row is that a retired `id` must never be handed to
a future account, which is what would let a stranger's events merge with a deleted person's.

Retention is not the same as retaining identity. `bad_problem_user` is the one event type a person
types prose into, so the same transaction scrubs `$.explanation` out of those rows (guarded by
`JSON_VALID`, since pre-JSON rows hold a bare problem id); what stays is the report, not the
reporter. Because the sentinel replaces the Auth0 `sub`, the same person logging in again
provisions a **fresh** row rather than re-claiming the anonymized one.

`TestDeleteAccount_NoUnpurgedUserTables` reads `information_schema` and fails if any table with a
`user_id` column is neither purged nor listed as retained — so a new per-user table can't silently
start leaking rows past deletion.

**Auth0 removal is best-effort.** After the local transaction commits, the handler calls
`auth0.DeleteUser` (Management API, client-credentials grant, against `managementDomain()` — see the
tenant-objects section above for why that is not `auth0_domain`) behind the optional config keys
`auth0_management_clientId` / `auth0_management_clientSecret`. Both unset is a normal dev
configuration and only logs; exactly one set is a misconfiguration and logs an error. A failure
never fails the request — our DB is already scrubbed, and an orphaned Auth0 identity just creates a
fresh row on next login.

**Client.** The delete card sits last in the settings grid (already behind the PIN gate), and its
confirmation modal re-asks for the PIN. On 204 the client clears the session PIN and logs out of
Auth0. Its copy has to stay truthful about the retained events: it says anonymous gameplay data is
kept, because saying "deletes everything" would be a promise this endpoint does not keep.

## Invariants

- **Role is never client-settable.** Neither create (`createUserSQL` omits `role`) nor update
  (`customUpdateUser` forces `model.Role` from the stored row) takes the role from request input.
  Promotion is DB-only.
- **A caller may only reach their own row.** Reads and writes alike: a path that names a user is
  403 unless that user is the caller, enforced once on the route group rather than per handler.
- **`RequireAdmin` and `RequireSelf` run after `UserMiddleware`.** Both depend on the loaded user;
  registering either earlier always 403s.
- **Admin surfaces are double-gated.** Server `RequireAdmin` (authoritative) + client `isAdmin`
  route guard (renders 404 to non-admins).
- **New rows default to `student` / empty PIN.** The empty PIN is the signal that drives a new
  account into the setup wizard.
- **A deleted account's `users` row is never removed, and its `id` is never handed out again.**
  That is what keeps retained `events` attributable to one retired account and no live one. The
  Auth0 `sub` itself *is* released: the sentinel overwrites it, so re-login provisions a fresh row.

## Gotchas / non-obvious behavior

- **The PIN is a UX gate, not a security boundary.** It is checked entirely in the browser against
  `user.pin`, which the client already holds (returned in the pageload payload). It exists to stop
  a *kid* from tapping into settings, not to authorize anything. All real authorization is the
  Auth0 JWT + role.
- **`RequirePin` is the first-stage guard, not `PinView`.** On `/settings`, `RequirePin(user.pin)`
  runs first and redirects to `/pin/...` unless the session PIN already equals `user.pin`; the
  `PinView` gate-mode check only runs after that redirect.
  Both compare against `user.pin` by equality (#274).
- **`ClearSessionPin` fires on several routes.** Rendering the 404 page, the home view, or the
  play view clears the session PIN (`index.js`, `home.js`, `play.js`), so leaving a protected area
  drops the gate.
- **The `/admin` group sits inside `authed`, so admin routes inherit `RequireSelf` too.** Harmless
  today (no admin route names a user), but an operator route that must read *another* account's data
  cannot live there: it would 403 for the operator. Such a route has to be registered outside
  `authed` and gate on `RequireAdmin`, and `selfOnlyRoutes` in `self_access_test.go` has to be taught
  to skip it — deliberately awkward, so opting a route out of self-only is a visible decision.
- **Two different enabled-video thresholds.** The setup gate re-shows when `numEnabledVideos < 3`
  even for an already-set-up account, while the wizard's final step only requires ≥ 1 enabled
  playlist/video — 3 to *exit* the gate, 1 to *finish* the wizard.

## Related files

- `server/api/roles.go` — `RoleStudent`, `RoleAdmin`, `RequireAdmin`, `adminWhoami`.
- `server/api/self_access.go` — `RequireSelf`, `selfOnlyPathParams`.
- `server/api/init.go` — Auth0 JWT + user-middleware wiring (`EnsureValidToken`,
  `Auth0IdMiddleware`, `UserMiddleware`); the `authed` and `/admin` group composition.
- `server/common/middleware.go` — `Auth0IdMiddleware`, `TestAuth0IdMiddleware`, `UserMiddleware`.
- `server/api/handler_helpers.go` — context accessors (`GetAuth0IdFromContext`,
  `GetUserFromContext`/`Lenient`).
- `server/api/custom_handlers.go` — `customUpdateUser`, `customCreateOrUpdateUser`.
- `server/api/delete_account.go` — `customDeleteAccount`, `anonymizeAndPurgeUser`,
  `perUserDeleteSQL`, `retainedUserTables`, `bestEffortDeleteAuth0User`.
- `server/common/auth0/management.go` — `DeleteUser`, `managementToken` (Management API client).
- `server/api/migrations/41.sql` — adds `users.role` (default `student`).
- `server/api/models.json` (`users` table) — `pin` and `role` fields; regenerate
  `user_model.generated.go` (which holds `createUserSQL`) via `make build-api`, never edit it.
- `web/src/index.js` — Auth0 provisioning, admin route guards, the setup gate.
- `web/src/auth0.js`, `web/src/pin.js`, `web/src/setup.js` — owned files.
