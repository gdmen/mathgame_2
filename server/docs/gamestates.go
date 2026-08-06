package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /gamestates/{user_id} gamestates getGamestate
Get the caller's own gamestate.
If the gamestate has no video assigned, one is selected and persisted before
the response is written.
responses:
  200: gamestateResp
  400: error
  403: error
  404: error
  500: error
*/

// swagger:parameters getGamestate
type gamestatePathParameters struct {
	// Id of the caller's own user row.
	// in:path
	// required: true
	UserId uint32 `json:"user_id"`
}

// swagger:response gamestateResp
type gamestateResponse struct {
	// in:body
	Body api.Gamestate
}
