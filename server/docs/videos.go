package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /videos videos listVideos
List the videos on the caller's own list.
responses:
  200: videoListResp
  500: error
*/

/*
swagger:route GET /videos/{id} videos getVideo
Get one video by id.
responses:
  200: videoResp
  400: error
  404: error
  500: error
*/

// swagger:parameters getVideo
type videoPathParameters struct {
	// in:path
	// required: true
	Id uint32 `json:"id"`
}

// swagger:response videoResp
type videoResponse struct {
	// in:body
	Body api.Video
}

// swagger:response videoListResp
type videoListResponse struct {
	// in:body
	Body []api.Video
}
