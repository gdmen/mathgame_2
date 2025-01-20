// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"fmt"
)

type Options struct {
	Features         []string `json:"features" form:"features"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`
	NumProblems      int      `json:"num_problems" form:"num_problems"`
}

func (opts Options) String() string {
	return fmt.Sprintf("Features: %v, TargetDifficulty: %v, NumProblems: %v", opts.Features, opts.TargetDifficulty, opts.NumProblems)
}
