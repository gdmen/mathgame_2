package mathcore

import (
	"reflect"
	"testing"
)

// TestProblemTypeBitInventory pins the bit layout: 16 bits, every bit named,
// masks consistent.
func TestProblemTypeBitInventory(t *testing.T) {
	if len(problemTypeNames) != 16 {
		t.Fatalf("bit inventory: %d names, want 16", len(problemTypeNames))
	}
	var all ProblemType
	for pt := range problemTypeNames {
		all |= pt
	}
	if all != ALL_PROBLEM_TYPES {
		t.Errorf("ALL_PROBLEM_TYPES = %d, OR of named bits = %d", ALL_PROBLEM_TYPES, all)
	}
}

func TestProblemTypeToFeatures(t *testing.T) {
	// Each single bit maps to its own name.
	for bit, name := range problemTypeNames {
		features := ProblemTypeToFeatures(bit)
		if len(features) != 1 || features[0] != name {
			t.Errorf("%q: got features %v", name, features)
		}
	}
	// A stack of bits emits every name, in ascending-bit (definition) order.
	features := ProblemTypeToFeatures(ADDITION | SUBTRACTION)
	if !reflect.DeepEqual(features, []string{"addition", "subtraction"}) {
		t.Errorf("multi: got %v, want [addition subtraction]", features)
	}
}
