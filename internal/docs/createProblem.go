package docs

import "garydmenezes.com/mathgame/internal/api"

/*
swagger:route POST /problems problems createProblem
Create a math problem.
responses:
  200: createProblemResp
*/

//swagger:response createProblemResp
type responseWrapper struct {
	//in:body
	Body api.Problem
}

//swagger:parameters createProblem
type parametersWrapper struct {
	//in:body
	Body api.CreateProblemReq
}
