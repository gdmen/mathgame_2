package docs

import "garydmenezes.com/mathgame/internal/api"
import "garydmenezes.com/mathgame/internal/generator"

/*
swagger:route POST /problems problems createProblem
Create a math problem.
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
