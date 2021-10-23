package docs

import "garydmenezes.com/mathgame/server/api"
import "garydmenezes.com/mathgame/server/generator"

/*
swagger:route POST /problems problems createProblem
Generate a problem.
responses:
  200: createProblemResp
  201: createProblemResp
  400: error
  500: error
*/

//swagger:parameters createProblem
type createProblemParameters struct {
	//in:body
	Body generator.Options
}

//swagger:response createProblemResp
type createProblemResponse struct {
	//in:body
	Body api.Problem
}
