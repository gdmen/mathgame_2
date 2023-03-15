// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

const (
	// EventTypes
	LOGGED_IN                  = "logged_in"                  // no value
	DISPLAYED_PROBLEM          = "displayed_problem"          // int ProblemID
	WORKING_ON_PROBLEM         = "working_on_problem"         // int Duration in seconds
	ANSWERED_PROBLEM           = "answered_problem"           // string Answer
	WATCHING_VIDEO             = "watching_video"             // int Duration in seconds
	DONE_WATCHING_VIDEO        = "done_watching_video"        // int VideoID
	SET_TARGET_DIFFICULTY      = "set_target_difficulty"      // float64 Difficulty
	SET_TARGET_WORK_PERCENTAGE = "set_target_work_percentage" // float64 Work Percentage
	SET_PROBLEM_TYPE_BITMAP    = "set_problem_type_bitmap"    // uint64 ProblemType Bitmap
	SET_GAMESTATE_TARGET       = "set_gamestate_target"       // uint32 Target num problems
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
