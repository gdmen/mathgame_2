// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

const (
	// EventTypes
	LOGGED_IN           = "logged_in"           // no value
	DISPLAYED_PROBLEM   = "displayed_problem"   // int ProblemID
	WORKING_ON_PROBLEM  = "working_on_problem"  // int Duration in seconds
	ANSWERED_PROBLEM    = "answered_problem"    // string Answer
	WATCHING_VIDEO      = "watching_video"      // int Duration in seconds
	DONE_WATCHING_VIDEO = "done_watching_video" // int VideoID
	// -end- EventTypes
)

const (
	// ProblemTypes are currently limited to 64 flags
	ADDITION uint64 = 1 << iota
	SUBTRACTION
	//MULTIPLICATION
	//DIVISION
	//FRACTIONS
	//NEGATIVES
	// -end- ProblemTypes
)
