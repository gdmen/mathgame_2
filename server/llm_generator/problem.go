// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"fmt"
)

// Problem is the narrator's output: prose plus the skeleton's authoritative
// SymbolicExpression + Answer, handed to the api side to answer/form-validate
// and store. It is never request-bound, so it carries no struct tags; the api
// computes and stores difficulty, so this struct holds none.
type Problem struct {
	Expression         string
	SymbolicExpression string
	Answer             string
	Explanation        string
}

func (opts Problem) String() string {
	return fmt.Sprintf("Expression: %v, SymbolicExpression: %v, Answer: %v, Explanation: %v", opts.Expression, opts.SymbolicExpression, opts.Answer, opts.Explanation)
}
