package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route POST /users users createOrUpdateUser
Create the caller's own user row, or return it if it already exists.
This is the first-login route. It names no user in its path and takes the
auth0_id from the token, because a first-login caller has no users row yet.
Answers 200 whether the row was created or already existed.
responses:
  200: userResp
  400: error
  500: error
*/

/*
swagger:route POST /users/{auth0_id} users updateUser
Update the caller's own user row.
Id and role are forced from the stored row, so neither can be set by the
client; the row written is always the token's own.
responses:
  200: userResp
  400: error
  403: error
  404: error
  500: error
*/

/*
swagger:route GET /users/{auth0_id} users getUser
Get the caller's own user row.
responses:
  200: userResp
  400: error
  403: error
  404: error
  500: error
*/

/*
swagger:route DELETE /users/{auth0_id} users deleteAccount
Delete the caller's own account.
Purges per-user state, anonymizes the users row, and best-effort removes the
Auth0 identity. Events are retained against the anonymized row. Requires the
adult PIN in the body as a deliberate-intent gate; 403 if it is wrong or if no
PIN has been set yet.
responses:
  204: emptyResp
  400: error
  403: error
  500: error
*/

// swagger:parameters createOrUpdateUser updateUser
type userBodyParameters struct {
	// in:body
	Body api.User
}

// swagger:parameters updateUser getUser deleteAccount
type userPathParameters struct {
	// Auth0 subject of the caller, which must match the token.
	// in:path
	// required: true
	Auth0Id string `json:"auth0_id"`
}

// swagger:parameters deleteAccount
type deleteAccountParameters struct {
	// in:body
	Body struct {
		// The adult PIN, re-entered to confirm.
		// required: true
		Pin string `json:"pin"`
	}
}

// swagger:response userResp
type userResponse struct {
	// in:body
	Body api.User
}

// swagger:response emptyResp
type emptyResponse struct {
}
