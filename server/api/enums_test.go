// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"reflect"
	"sort"
	"testing"
)

func TestGetProblemTypePermutations(t *testing.T) {
	cases := map[ProblemType][]ProblemType{
		1:  {1},
		2:  {2},
		3:  {1, 2, 3},
		4:  {4},
		5:  {1, 4, 5},
		6:  {2, 4, 6},
		7:  {1, 2, 3, 4, 5, 6, 7},
		8:  {8},
		9:  {1, 8, 9},
		10: {2, 8, 10},
		11: {1, 2, 3, 8, 9, 10, 11},
		12: {4, 8, 12},
		13: {1, 4, 5, 8, 9, 12, 13},
		14: {2, 4, 6, 8, 10, 12, 14},
		15: {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	}
	for k, v := range cases {
		res := GetProblemTypePermutations(k)
		sort.Slice(res, func(i, j int) bool {
			return res[i] < res[j]
		})
		if !reflect.DeepEqual(res, v) {
			t.Errorf("ProblemTypePermutations(%d) = %v, want %v", k, res, v)
		}
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
