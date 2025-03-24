// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

const (
	// EventTypes
	LOGGED_IN                  = "logged_in"                  // no value
	SELECTED_PROBLEM           = "selected_problem"           // int ProblemID
	WORKING_ON_PROBLEM         = "working_on_problem"         // int Duration in seconds
	ANSWERED_PROBLEM           = "answered_problem"           // string Answer
	SOLVED_PROBLEM             = "solved_problem"             // int ProblemID
	ERROR_PLAYING_VIDEO        = "error_playing_video"        // string Error
	WATCHING_VIDEO             = "watching_video"             // int Duration in seconds
	DONE_WATCHING_VIDEO        = "done_watching_video"        // int VideoID
	SET_TARGET_DIFFICULTY      = "set_target_difficulty"      // float64 Difficulty
	SET_TARGET_WORK_PERCENTAGE = "set_target_work_percentage" // float64 Work Percentage
	SET_PROBLEM_TYPE_BITMAP    = "set_problem_type_bitmap"    // uint64 ProblemType Bitmap
	SET_GAMESTATE_TARGET       = "set_gamestate_target"       // uint32 Target num problems
	BAD_PROBLEM_SYSTEM         = "bad_problem_system"         // int ProblemID
	BAD_PROBLEM_USER           = "bad_problem_user"           // int ProblemID
	// -end- EventTypes
)

type ProblemType uint64

const (
	// ProblemTypes are currently limited to 64 flags
	ADDITION ProblemType = 1 << iota
	SUBTRACTION
	MULTIPLICATION
	DIVISION
	FRACTIONS
	NEGATIVES
	WORD
	// -end- ProblemTypes
)

// Map to associate ProblemType values with string names
var problemTypeNames = map[ProblemType]string{
	ADDITION:       "addition",
	SUBTRACTION:    "subtraction",
	MULTIPLICATION: "multiplication",
	DIVISION:       "division",
	FRACTIONS:      "fractions",
	NEGATIVES:      "negatives",
	WORD:           "word",
}

// Map to associate string names with ProblemType values
var problemTypeValues = map[string]ProblemType{
	"addition":       ADDITION,
	"subtraction":    SUBTRACTION,
	"multiplication": MULTIPLICATION,
	"division":       DIVISION,
	"fractions":      FRACTIONS,
	"negatives":      NEGATIVES,
	"word":           WORD,
}

// Convert a ProblemType Bitmap into an array of string features
func ProblemTypeToFeatures(pt ProblemType) []string {
	features := []string{}
	for k, v := range problemTypeNames {
		if (k & pt) > 0 {
			features = append(features, v)
		}
	}
	return features
}

// Convert an array of string features into a ProblemType Bitmap
func FeaturesToProblemType(features []string) ProblemType {
	pt := ProblemType(0)
	for _, v := range features {
		pt |= problemTypeValues[v]
	}
	return pt
}

// Convert a ProblemType Bitmap into a list of all permutations
// e.g. pt = 5 -> 101 in binary -> [1,4,5]
func GetProblemTypePermutations(pt ProblemType) []ProblemType {
	var positions []int

	// Find all positions of 1 bits in pt
	for i := 0; i < 64; i++ {
		if (pt & (1 << i)) != 0 {
			positions = append(positions, i)
		}
	}

	return getProblemTypePermutationsHelper(ProblemType(0), positions, 0)
}

func getProblemTypePermutationsHelper(p ProblemType, positions []int, i int) []ProblemType {
	if i >= len(positions) {
		if p == 0 {
			return []ProblemType{}
		}
		return []ProblemType{p}
	}
	p1 := getProblemTypePermutationsHelper(p, positions, i+1)
	p2 := getProblemTypePermutationsHelper(p|ProblemType(1<<positions[i]), positions, i+1)
	return append(p1, p2...)
}
