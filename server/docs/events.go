package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route POST /events events createEvent
Record one gameplay event and get back the resulting play data.
The event drives the gamestate machine, so the response is the caller's
post-event play data (gamestate, next problem, next video) rather than the
stored event. Reporting a bad problem marks it reported so it stops being
served; if it was the current problem, the same response carries its
replacement.
responses:
  200: playDataResp
  400: error
  404: error
  500: error
*/

/*
swagger:route GET /events/{user_id}/{seconds} events listEvents
List the caller's own recent events.
Limited to the last {seconds} seconds and to the event types the client
replays: LOGGED_IN, SELECTED_PROBLEM, ANSWERED_PROBLEM, SOLVED_PROBLEM,
DONE_WATCHING_VIDEO.
responses:
  200: eventListResp
  400: error
  403: error
  500: error
*/

// swagger:parameters createEvent
type createEventParameters struct {
	// in:body
	Body api.Event
}

// swagger:parameters listEvents
type listEventsParameters struct {
	// Id of the caller's own user row.
	// in:path
	// required: true
	UserId uint32 `json:"user_id"`
	// How far back to look, in seconds.
	// in:path
	// required: true
	Seconds uint32 `json:"seconds"`
}

// swagger:response eventListResp
type eventListResponse struct {
	// in:body
	Body []api.Event
}
