package api

import "testing"

func TestEventTypeConstants(t *testing.T) {
	eventTypes := []string{
		LOGGED_IN, SELECTED_PROBLEM, WORKING_ON_PROBLEM, ANSWERED_PROBLEM, SOLVED_PROBLEM,
		ERROR_PLAYING_VIDEO, WATCHING_VIDEO, DONE_WATCHING_VIDEO,
		SET_TARGET_DIFFICULTY, SET_TARGET_WORK_PERCENTAGE, SET_PROBLEM_TYPE_BITMAP,
		SET_GAMESTATE_TARGET, BAD_PROBLEM_SYSTEM, BAD_PROBLEM_USER,
	}
	seen := make(map[string]bool)
	for _, et := range eventTypes {
		if et == "" {
			t.Errorf("event type constant is empty")
		}
		if seen[et] {
			t.Errorf("duplicate event type %q", et)
		}
		seen[et] = true
	}
}

func TestIsRecordOnlyEvent(t *testing.T) {
	tests := []struct {
		eventType string
		want      bool
	}{
		{LOGGED_IN, true},
		{WORKING_ON_PROBLEM, true},
		{WATCHING_VIDEO, true},
		{SET_TARGET_WORK_PERCENTAGE, true},
		{SELECTED_PROBLEM, false},
		{ANSWERED_PROBLEM, false},
		{SOLVED_PROBLEM, false},
		{ERROR_PLAYING_VIDEO, false},
		{DONE_WATCHING_VIDEO, false},
		{SET_TARGET_DIFFICULTY, false},
		{SET_PROBLEM_TYPE_BITMAP, false},
		{SET_GAMESTATE_TARGET, false},
		{BAD_PROBLEM_SYSTEM, false},
		{BAD_PROBLEM_USER, false},
		{"invalid_event_type", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			if got := isRecordOnlyEvent(tt.eventType); got != tt.want {
				t.Errorf("isRecordOnlyEvent(%q) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}
