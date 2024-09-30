// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"fmt"
)

type Options struct {
	Features            []string `json:"features" form:"features"`
	TargetDifficulty    float64  `json:"target_difficulty" form:"target_difficulty"`
	PreviousExpressions []string `json:"previous_expressions" form:"previous_expressions"`
	NumProblems         int      `json:"num_problems" form:"num_problems"`
}

func (opts Options) String() string {
	return fmt.Sprintf("Features: %v, TargetDifficulty: %v, PreviousExpressions: %v, NumProblems: %v", opts.Features, opts.TargetDifficulty, opts.PreviousExpressions, opts.NumProblems)
}
