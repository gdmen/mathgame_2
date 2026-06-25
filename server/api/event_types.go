// event_types.go: the event-type vocabulary and the record-only classifier.
// The event-type inventory is documented in docs/events.md (pinned by
// TestDocsSyncEvents).
package api

const (
	// EventTypes
	LOGGED_IN                  = "logged_in"                  // no value
	SELECTED_PROBLEM           = "selected_problem"           // int ProblemID
	WORKING_ON_PROBLEM         = "working_on_problem"         // int Duration in milliseconds
	ANSWERED_PROBLEM           = "answered_problem"           // string Answer
	SOLVED_PROBLEM             = "solved_problem"             // int ProblemID
	ERROR_PLAYING_VIDEO        = "error_playing_video"        // string Error
	WATCHING_VIDEO             = "watching_video"             // int Duration in milliseconds
	DONE_WATCHING_VIDEO        = "done_watching_video"        // int VideoID
	SET_TARGET_DIFFICULTY      = "set_target_difficulty"      // float64 Difficulty
	SET_TARGET_WORK_PERCENTAGE = "set_target_work_percentage" // float64 Work Percentage
	SET_PROBLEM_TYPE_BITMAP    = "set_problem_type_bitmap"    // uint64 ProblemType Bitmap
	SET_GAMESTATE_TARGET       = "set_gamestate_target"       // uint32 Target num problems
	BAD_PROBLEM_SYSTEM         = "bad_problem_system"         // int ProblemID
	BAD_PROBLEM_USER           = "bad_problem_user"           // int ProblemID
	// -end- EventTypes
)

// recordOnlyEventTypes are events that only need to be persisted; they do not
// mutate gamestate or settings. Use the simple processRecordOnlyEvents path.
var recordOnlyEventTypes = map[string]bool{
	LOGGED_IN:                  true,
	WORKING_ON_PROBLEM:         true,
	WATCHING_VIDEO:             true,
	SET_TARGET_WORK_PERCENTAGE: true,
}

func isRecordOnlyEvent(eventType string) bool {
	return recordOnlyEventTypes[eventType]
}
