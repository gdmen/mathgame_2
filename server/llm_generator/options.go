// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

type Options struct {
	Features         []string `json:"features" form:"features"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`
	NumProblems      int      `json:"num_problems" form:"num_problems"`
	// Constraints is the rendered MAY / MUST NOT block built from the user's
	// settings bitmap (api.BuildBitConstraints). The api side owns bit
	// semantics; this package treats the block as opaque prompt text.
	Constraints string `json:"constraints" form:"constraints"`
}
