// Package gpt_generator contains a math problem gpt_generator
package gpt_generator // import "garydmenezes.com/mathgame/server/gpt_generator"

import (
	"fmt"
)

type OptionsError struct {
	s string
}

func (e *OptionsError) Error() string {
	return e.s
}

type Options struct {
	Operations       []string `json:"operations" form:"operations"`
	Fractions        bool     `json:"fractions" form:"fractions"`
	Negatives        bool     `json:"negatives" form:"negatives"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`
}

func (opts Options) String() string {
	return fmt.Sprintf("Operations: %s", opts.Operations)
}

func validateOptions(opts *Options) error {
	for _, o := range opts.Operations {
		if !isSupportedOperation(o) {
			return &OptionsError{s: fmt.Sprintf("'%s' is not a supported operation", o)}
		}
	}
	return nil
}
