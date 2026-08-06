package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /settings/{user_id} settings getSettings
Get the caller's own settings.
responses:
  200: settingsResp
  400: error
  403: error
  404: error
  500: error
*/

/*
swagger:route POST /settings/{user_id} settings updateSettings
Update the caller's own settings.
user_id is taken from the authenticated identity, not the path or body.
target_difficulty is clamped into the band the saved problem_type_bitmap
supports before the write, so the value returned may differ from the one sent.
Changed fields also emit settings events.
responses:
  200: settingsResp
  400: error
  403: error
  404: error
  500: error
*/

// swagger:parameters getSettings updateSettings
type settingsPathParameters struct {
	// Id of the caller's own user row.
	// in:path
	// required: true
	UserId uint32 `json:"user_id"`
}

// swagger:parameters updateSettings
type updateSettingsParameters struct {
	// in:body
	Body api.Settings
}

// swagger:response settingsResp
type settingsResponse struct {
	// in:body
	Body api.Settings
}
