// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"reflect"
	"sort"
	"testing"
)

// TestProblemTypeBitInventory pins the bit layout: 16 bits, every bit named,
// every name mapped back, masks consistent.
func TestProblemTypeBitInventory(t *testing.T) {
	if len(problemTypeNames) != 16 || len(problemTypeValues) != 16 {
		t.Fatalf("bit inventory: %d names, %d values, want 16 each",
			len(problemTypeNames), len(problemTypeValues))
	}
	var all ProblemType
	for pt, name := range problemTypeNames {
		if problemTypeValues[name] != pt {
			t.Errorf("name map mismatch: %s -> %d, want %d", name, problemTypeValues[name], pt)
		}
		all |= pt
	}
	if all != ALL_PROBLEM_TYPES {
		t.Errorf("ALL_PROBLEM_TYPES = %d, OR of named bits = %d", ALL_PROBLEM_TYPES, all)
	}
	// WEIGHTED_TOPIC_MASK is exactly the non-magnitude bits.
	if WEIGHTED_TOPIC_MASK != ALL_PROBLEM_TYPES&^(MEDIUM_NUMBERS|LARGE_NUMBERS) {
		t.Errorf("WEIGHTED_TOPIC_MASK = %d, want all bits except magnitude", WEIGHTED_TOPIC_MASK)
	}
}

func TestProblemTypeToFeaturesRoundTrip(t *testing.T) {
	names := []string{"addition", "subtraction", "multiplication", "division", "fractions", "negatives", "word"}
	for _, name := range names {
		pt := FeaturesToProblemType([]string{name})
		features := ProblemTypeToFeatures(pt)
		if len(features) != 1 || features[0] != name {
			t.Errorf("roundtrip %q: got features %v", name, features)
		}
	}
	// Multiple features
	pt := FeaturesToProblemType([]string{"addition", "subtraction"})
	features := ProblemTypeToFeatures(pt)
	sort.Strings(features)
	if !reflect.DeepEqual(features, []string{"addition", "subtraction"}) {
		t.Errorf("multi roundtrip: got %v", features)
	}
	pt2 := FeaturesToProblemType(features)
	if pt != pt2 {
		t.Errorf("roundtrip pt mismatch: %d vs %d", pt, pt2)
	}
}

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
