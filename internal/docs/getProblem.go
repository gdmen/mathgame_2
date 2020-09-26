package docs

import "garydmenezes.com/mathgame/internal/api"

/*
swagger:route GET /problems/{id} problems getProblem
Get a problem.
responses:
  200: getProblemResp
  400: error
  404: error
  500: error
*/

//swagger:parameters getProblem
type getProblemParameters struct {
	//in:path
	Id uint64 `json:"id"`
}

//swagger:response getProblemResp
type getProblemResponse struct {
	//in:body
	Body api.Problem
}
