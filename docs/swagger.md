# The Swagger API spec

How the machine-readable API contract is written, built, and served. This doc owns the
*mechanism*; the contract itself is the generated `server/docs/spec/swagger.yaml`, which is the API
documentation and is not restated here. **Change this doc in the same PR as any behavior change here.** This area is prose
(no doc-sync anchors); `make docs-check BASE=origin/master` flags a PR that touches the owned
files without touching this doc.

Owned files: `server/docs/**`.

## The model

`server/docs` is documentation-only in intent, but it is a real Go package and
`cmd/apiserver/main.go` blank-imports it, so it links into `bin/apiserver`. That is the useful
part: a compile error in `server/docs` fails `make build-api`, and therefore `make test`. An
annotation referencing a type that was renamed or deleted breaks the build instead of silently
producing a wrong spec.

Definitions in the spec are derived from the live Go types the annotations point at; the
table-backed ones (`User`, `Problem`, `Settings`, `Gamestate`, `Event`, `Video`) trace to
`server/api/models.json` via `make build-api`. The rest are hand-written response structs such as
`PageLoadData`, `PlayData`, and the admin report types. Either way the fields are not restated in
the annotations, so no field list can drift. Only the routes and their status codes are
hand-written, so those are the only things that can go stale.

Nothing compares the `swagger:route` blocks against the router, so a route added to or dropped
from `server/api/init.go` has to be mirrored here in the same change — the spec would otherwise
keep advertising an endpoint that answers 404. A `swagger:parameters` struct naming only that
operation goes with it. The compiler is no help here: it checks the parameter and response
*structs* (a renamed `api.Video` field breaks the build), but the annotations themselves are
comments, so an operation name that no longer corresponds to any route compiles cleanly.

`make build-docs` scans the annotations into `server/docs/spec/swagger.yaml`, merging
`server/docs/swagger_base.yml`, then validates the result — and `build-api` runs it before
compiling `apiserver`, so the spec has the same life cycle as `enums.generated.js`: any build
regenerates it in your tree and you commit what `git status` shows, never running the target by
hand. It depends on `check-swagger`, which builds go-swagger into `bin/swagger` at the version
go.mod pins (recorded in `tools.go`), so every checkout regenerates the same bytes and the output
is deterministic — a go-swagger version bump rewrites the file wholesale (ordering, formatting,
definition shapes) and belongs in its own commit with the regenerate. The UI for reading the spec
is the admin page below; there is no standalone viewer target.

## The served spec

`swagger.yaml` is **generated but committed**, because `apiserver` embeds it: `server/docs/spec` is
a leaf package (`//go:embed swagger.yaml`, no imports of its own) that `server/api` can import
without the cycle `server/docs` → `server/api` would create. `adminSwaggerSpec`
(`server/api/admin_swagger.go`) serves it at `GET /admin/swagger.yaml` inside the `/admin` group,
so it is readable only with an admin's token ([accounts.md](accounts.md) owns the gate). The admin
home links to `/admin/api-docs` (`web/src/admin_api_docs.js`), which lazy-loads `swagger-ui-dist`
and points it at that URL with a `requestInterceptor` that adds the bearer token. The same
interceptor covers the UI's "Try it out" calls, so they run as the signed-in admin against the
live API with no Authorize step. (`swagger-ui-dist` pulls in `@scarf/scarf`, whose `postinstall`
reports to scarf.sh on every `npm ci`; set `SCARF_ANALYTICS=false` in an environment that should
not.)

Consequences for the annotations:

- **No `Host` or `Schemes` in the meta.** Swagger 2.0 falls back to the host and scheme the spec
  was loaded from, which is exactly the API the page should call in both dev and prod. Putting a
  host back would send "Try it out" somewhere else; putting `http` first in schemes would send prod
  POSTs through nginx's 301 and turn them into GETs.
- **Committing the regenerated file is part of documenting a route.** Two gates enforce it. CI's
  build step regenerates the spec and a diff step fails on any mismatch with the committed file,
  which catches every kind of staleness including definition drift from renamed fields.
  `TestDocsSyncSwaggerSpec` (`server/api/docs_sync_test.go`) compares the `swagger:route`
  annotations (the flush-left block-comment form, in this package — the only form used here)
  against the committed file's operations in both directions — a weaker check, but it needs no
  go-swagger, so drift surfaces in any `make test`. The committed file sits under this area's
  globs, so a regenerate alone counts as a swagger-area change for `make docs-check`; touch this
  doc when that happens.
- **A primitive response body is declared in `swagger_base.yml`, not Go.** go-swagger drops the
  schema of a `swagger:response` whose body is a bare `string`; `swaggerSpecResp` lives in the
  base file for that reason.

## The formatting constraint

**Indented lines in the `swagger:meta` block break the build.** go-swagger reads `swagger:meta`
only from the package doc comment (`server/docs/docs.go`), gofmt rewrites indented doc-comment
lines to tab indentation, and YAML rejects tabs as indentation. go-swagger's own indent stripper
matches `\p{Zs}`, which does not include tab. So a doc comment carrying indented YAML fails with
`yaml: found character that cannot start any token`, and `make fmt` is enough to cause it.

Two rules follow, and both are load-bearing:

- **The meta keys stay flush left**, so gofmt has nothing to reindent.
- **Anything needing nesting lives in `server/docs/swagger_base.yml`**, merged in with
  `swagger generate spec -i`. That is where `securityDefinitions` and the global `security` entry
  are, and it is a plain `.yml` file that gofmt never touches.

Per-operation `swagger:route` blocks are *free-floating* comments, not doc comments, and gofmt
leaves those alone, so they keep ordinary two-space YAML indentation. The distinction is only
whether a comment is attached to a declaration: put a blank line between the block and the type
below it, as the existing files do.

After changing anything in `server/docs`, run `make fmt && make build-docs`. Passing once is not
enough; it has to pass *after* a format pass.

## Layout

One file per resource, named for it (`users.go`, `videos.go`, `playlists.go`, `admin.go`, …), each
holding every operation on that resource plus the parameter and response types those operations
need. `docs.go` holds the spec metadata and the shared `error` response.

Shared conventions worth keeping: a `swagger:parameters` struct may list several operation ids, so
one path-parameter type serves every operation on the resource; `emptyResp` is the shared 204.

## Gotchas

- **A route with no user-naming path param is not self-gated.** `RequireSelf` only compares params
  it finds, so `POST /events` and the collection routes cannot answer 403. Documenting one there is
  a factual error.
- **The middleware answers before the handler does.** A valid token with no `users` row yields 404,
  not the 401 the handlers' `user == nil` guards suggest; [accounts.md](accounts.md) owns that
  layer. Status codes copied from a handler's own error paths will be wrong without it.

## Extension checklist — documenting a route

The router is `GetRouter` in `server/api/init.go`, and it is the list to check against.
Trailing-slash duplicate registrations share one entry.

1. Add the `swagger:route` block to the resource's file, or a new file if it is a new resource.
   The path is relative to the `/api/v1` base path.
2. List the status codes the handler can **actually** return. Read the handler, do not assume the
   CRUD set. Codes that cannot happen are worse than missing ones: they are the spec lying.
3. Leave the middleware-wide codes to the `docs.go` meta description, which states 401, the
   no-users-row 404, and 403 once. The per-operation 404s are the resource's own.
4. Point response bodies at the live API types rather than restating fields, so the definitions
   stay generated.
5. Run `make fmt && make build-docs`, and commit the regenerated `server/docs/spec/swagger.yaml`
   with the annotation.

## Related files

- `server/docs/docs.go` — `swagger:meta` (base path, version, the global auth note) and the shared
  `error` response.
- `server/docs/swagger_base.yml` — `securityDefinitions`, the global `security` entry, and the
  `swaggerSpecResp` response.
- `server/docs/spec/` — the committed `swagger.yaml` and the package that embeds it.
- `server/api/admin_swagger.go` — serves the embedded spec; `server/api/docs_sync_test.go`
  (`TestDocsSyncSwaggerSpec`) pins the committed file to the route annotations.
- `web/src/admin_api_docs.js` — the admin API docs page (Swagger UI over the served spec).
- `cmd/apiserver/main.go` — blank-imports `server/docs`, which is what makes a broken annotation a
  build failure.
- `server/api/init.go` — `GetRouter`, the authoritative route list, and the middleware chain whose
  status codes the annotations describe.
- `Makefile` — `check-swagger`, `build-docs`.
- `tools.go` — records the go-swagger dependency that pins the version in `go.mod`.
- [docs/accounts.md](accounts.md) — what the auth layers actually enforce, behind the 401/403/404
  the spec reports.
- [docs/ops-runbook.md](ops-runbook.md) — the Makefile targets in their build context.
