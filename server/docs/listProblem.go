package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /problems problems listProblem
List all problems.
responses:
  200: listProblemResp
  500: error
*/

//swagger:response listProblemResp
type listProblemResponse struct {
	//in:body
	Body []api.Problem
}
