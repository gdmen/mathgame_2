package docs

import "garydmenezes.com/mathgame/server/api"

/*
swagger:route GET /playlists playlists listPlaylists
List the playlists the caller has added, with per-playlist video tallies.
playable_total is the de-duplicated count across all of them: summing
playable_count per playlist double-counts a video that sits in two, so callers
must not compute the total themselves.
responses:
  200: myPlaylistsResp
  500: error
*/

/*
swagger:route POST /playlists playlists addPlaylist
Add a playlist to the caller's list.
Identify the playlist by playlist_id (an already-known row),
youtube_playlist_id, or playlist_url, checked in that order; the latter two are
fetched from YouTube on first use. Returns the id of the playlist that was
added.
responses:
  200: addPlaylistResp
  400: error
  404: error
  500: error
*/

/*
swagger:route GET /playlists/{playlist_id}/videos playlists listPlaylistVideos
List the videos in one playlist the caller has added.
The caller's playlist membership is the authorization: a playlist they have not
added returns no rows.
responses:
  200: videoListResp
  400: error
  500: error
*/

/*
swagger:route DELETE /playlists/{playlist_id} playlists removePlaylist
Remove a playlist from the caller's list.
responses:
  204: emptyResp
  400: error
  404: error
  500: error
*/

// swagger:parameters listPlaylistVideos removePlaylist
type playlistPathParameters struct {
	// in:path
	// required: true
	PlaylistId uint32 `json:"playlist_id"`
}

// swagger:parameters addPlaylist
type addPlaylistParameters struct {
	// in:body
	Body struct {
		// Id of a playlist row already known to the server.
		PlaylistId *uint32 `json:"playlist_id"`
		// YouTube's own playlist id.
		YouTubePlaylistId *string `json:"youtube_playlist_id"`
		// A YouTube playlist URL to parse the id out of.
		PlaylistUrl *string `json:"playlist_url"`
	}
}

// swagger:response myPlaylistsResp
type myPlaylistsResponse struct {
	// in:body
	Body api.MyPlaylists
}

// swagger:response addPlaylistResp
type addPlaylistResponse struct {
	// in:body
	Body struct {
		Id uint32 `json:"id"`
	}
}
