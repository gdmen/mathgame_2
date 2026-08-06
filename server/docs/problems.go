package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /problems/{id} problems getProblem
Get one problem by id.
A problem row belongs to no user, so this route needs a valid token but no
users row behind it.
responses:
  200: getProblemResp
  400: error
  404: error
  500: error
*/

// swagger:parameters getProblem
type getProblemParameters struct {
	// in:path
	// required: true
	Id uint32 `json:"id"`
}

// swagger:response getProblemResp
type getProblemResponse struct {
	// in:body
	Body api.Problem
}
