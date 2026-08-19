// Package classification Math Game API.
//
// Every route requires an Auth0-issued bearer JWT and answers 401 without a
// valid one. Routes carrying an :auth0_id or :user_id path parameter are
// further restricted to the caller's own record, and /admin routes additionally
// require the admin role, so both can answer 403 for an authenticated caller.
// Every route except POST /users and GET /problems/{id} also answers 404 when
// the token is valid but has no users row behind it yet; the 404s listed on
// each operation below are that resource's own.
//
// BasePath: /api/v1
// Version: 1.0.0
//
// Consumes:
// - application/json
//
// Produces:
// - application/json
//
// swagger:meta
package docs

// The keys above are flush left, and nested YAML lives in swagger_base.yml, for
// the reason documented in docs/swagger.md.

import "garydmenezes.com/mathgame/server/common"

// swagger:response error
type ErrorResp struct {
	// in:body
	Body common.Error
}
