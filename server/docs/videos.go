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
swagger:route POST /videos videos createVideo
Add a video to the caller's own list, creating the catalog row if needed.
responses:
  200: videoResp
  201: videoResp
  400: error
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

/*
swagger:route DELETE /videos/{id} videos deleteVideo
Remove a video from the caller's own list.
The catalog row is never deleted, so event and gamestate references to it stay
valid. 404 when the video is not on the caller's list.
responses:
  204: emptyResp
  400: error
  404: error
  500: error
*/

// swagger:parameters getVideo deleteVideo
type videoPathParameters struct {
	// in:path
	// required: true
	Id uint32 `json:"id"`
}

// swagger:parameters createVideo
type createVideoParameters struct {
	// in:body
	Body api.Video
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
