package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /pageload/{auth0_id} session getPageLoadData
Get everything the client needs on page load.
One call for the caller's user row, their settings, and how many videos they
have enabled.
responses:
  200: pageLoadResp
  400: error
  403: error
  404: error
  500: error
*/

/*
swagger:route GET /play/{user_id} session getPlayData
Get the caller's current gamestate with its problem and video resolved.
Selects and persists a problem or video when the gamestate is missing one. 403
when the caller has too few playable videos to start a session; the message
names the minimum.
responses:
  200: playDataResp
  400: error
  403: error
  404: error
  500: error
*/

/*
swagger:route GET /statistics/{user_id} session getStatistics
Get the caller's own lifetime and per-month totals.
Reads through the statistics cache, refreshing it from any events recorded
since the last read before answering.
responses:
  200: statisticsResp
  403: error
  500: error
*/

// swagger:parameters getPageLoadData
type pageLoadParameters struct {
	// Auth0 subject of the caller, which must match the token.
	// in:path
	// required: true
	Auth0Id string `json:"auth0_id"`
}

// swagger:parameters getPlayData getStatistics
type playDataParameters struct {
	// Id of the caller's own user row.
	// in:path
	// required: true
	UserId uint32 `json:"user_id"`
}

// swagger:response pageLoadResp
type pageLoadResponse struct {
	// in:body
	Body api.PageLoadData
}

// swagger:response playDataResp
type playDataResponse struct {
	// in:body
	Body api.PlayData
}

// swagger:response statisticsResp
type statisticsResponse struct {
	// in:body
	Body api.StatisticsResponse
}
