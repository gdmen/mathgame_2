package docs

/*
swagger:route DELETE /problems/{id} problems deleteProblem
Delete a math problem.
responses:
  204:
  400: error
  404: error
  500: error
*/

//swagger:parameters deleteProblem
type deleteProblemParameters struct {
	//in:path
	Id uint64 `json:"id"`
}
