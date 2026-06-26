// problem_type.go: the problem-type bit inventory and its name<->bit maps.

package mathcore

type ProblemType uint64

const (
	// ProblemTypes are currently limited to 64 flags.
	// Bits are named for the detectable expression feature, not the
	// curriculum subject.
	ADDITION ProblemType = 1 << iota
	SUBTRACTION
	MULTIPLICATION
	DIVISION
	FRACTIONS
	NEGATIVES
	WORD
	// Magnitude bits. Default (no bits) = max operand <= 12. Monotonic:
	// LARGE_NUMBERS requires MEDIUM_NUMBERS (no 13-99 gap). maxMagnitude is
	// digit-based for decimals (0.75 counts as 75).
	MEDIUM_NUMBERS // maxMagnitude 13-99
	LARGE_NUMBERS  // maxMagnitude >= 100
	// Chain bit. Default (off) = single operation.
	CHAINED_OPERATIONS // numOps >= 2
	// Concept bits.
	MISSING_NUMBER          // a single '?' blank outside \text{}; bare lone variables are rewritten into this form
	MISMATCHED_DENOMINATORS // fractions with differing denominators; requires FRACTIONS
	DECIMALS
	PEMDAS          // requires non-left-to-right evaluation (dual-eval rule); requires CHAINED_OPERATIONS
	SINGLE_VARIABLE // coefficient and/or multi-occurrence variable letter (load-bearing algebra notation)
	PERCENTAGES
	// -end- ProblemTypes
)

// ALL_PROBLEM_TYPES is every defined bit; values outside it are invalid.
const ALL_PROBLEM_TYPES ProblemType = (PERCENTAGES << 1) - 1

// WEIGHTED_TOPIC_MASK gates which bits participate in weighted topic
// selection and per-topic stats (chooseWeightedTopic, recordTopicAttempt,
// initTopicStats). A bit belongs iff per-topic difficulty coheres for it:
// "slightly more of this, but easier" must be meaningful. Magnitude bits are
// excluded because magnitude IS difficulty - "weak at LARGE_NUMBERS -> serve
// large numbers, easier" fights itself; size progression is
// target_difficulty's job. Deliberately decoupled from UI groupings.
const WEIGHTED_TOPIC_MASK ProblemType = ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION |
	FRACTIONS | NEGATIVES | WORD | CHAINED_OPERATIONS | MISSING_NUMBER |
	MISMATCHED_DENOMINATORS | DECIMALS | PEMDAS | SINGLE_VARIABLE | PERCENTAGES

// Map to associate ProblemType values with string names
var problemTypeNames = map[ProblemType]string{
	ADDITION:                "addition",
	SUBTRACTION:             "subtraction",
	MULTIPLICATION:          "multiplication",
	DIVISION:                "division",
	FRACTIONS:               "fractions",
	NEGATIVES:               "negatives",
	WORD:                    "word",
	MEDIUM_NUMBERS:          "medium_numbers",
	LARGE_NUMBERS:           "large_numbers",
	CHAINED_OPERATIONS:      "chained_operations",
	MISSING_NUMBER:          "missing_number",
	MISMATCHED_DENOMINATORS: "mismatched_denominators",
	DECIMALS:                "decimals",
	PEMDAS:                  "pemdas",
	SINGLE_VARIABLE:         "single_variable",
	PERCENTAGES:             "percentages",
}

// Map to associate string names with ProblemType values
var problemTypeValues = map[string]ProblemType{
	"addition":                ADDITION,
	"subtraction":             SUBTRACTION,
	"multiplication":          MULTIPLICATION,
	"division":                DIVISION,
	"fractions":               FRACTIONS,
	"negatives":               NEGATIVES,
	"word":                    WORD,
	"medium_numbers":          MEDIUM_NUMBERS,
	"large_numbers":           LARGE_NUMBERS,
	"chained_operations":      CHAINED_OPERATIONS,
	"missing_number":          MISSING_NUMBER,
	"mismatched_denominators": MISMATCHED_DENOMINATORS,
	"decimals":                DECIMALS,
	"pemdas":                  PEMDAS,
	"single_variable":         SINGLE_VARIABLE,
	"percentages":             PERCENTAGES,
}

// Convert a ProblemType Bitmap into an array of string features. Bits are
// emitted in definition (ascending bit) order so the result is deterministic —
// a map range would shuffle it per call.
func ProblemTypeToFeatures(pt ProblemType) []string {
	features := []string{}
	for bit := ProblemType(1); bit <= ALL_PROBLEM_TYPES; bit <<= 1 {
		if pt&bit != 0 {
			features = append(features, problemTypeNames[bit])
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
