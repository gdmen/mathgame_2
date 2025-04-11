// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

type PageLoadData struct {
	User             *User       `json:"user"`
	Settings         *Settings   `json:"settings"`
	NumVideosEnabled interface{} `json:"num_videos_enabled"`
}

type PlayData struct {
	Gamestate *Gamestate `json:"gamestate"`
	Problem   *Problem   `json:"problem"`
	Video     *Video     `json:"video"`
}
