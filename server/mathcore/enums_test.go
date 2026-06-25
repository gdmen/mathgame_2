package mathcore

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
