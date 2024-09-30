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
