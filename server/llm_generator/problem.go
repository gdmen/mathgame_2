// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"fmt"
)

type Problem struct {
	Features           []string `json:"features" form:"features"`
	Expression         string   `json:"expression" form:"expression"`
	SymbolicExpression string   `json:"symbolic_expression" form:"symbolic_expression"`
	Answer             string   `json:"answer" form:"answer"`
	Explanation        string   `json:"explanation" form:"explanation"`
	Difficulty         float64  `json:"difficulty" form:"difficulty"`
}

func (opts Problem) String() string {
	return fmt.Sprintf("Features: %v, Expression: %v, SymbolicExpression: %v, Answer: %v, Explanation: %v, Difficulty: %v", opts.Features, opts.Expression, opts.SymbolicExpression, opts.Answer, opts.Explanation, opts.Difficulty)
}
