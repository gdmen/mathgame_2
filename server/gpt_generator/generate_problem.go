// Package gpt_generator contains a math problem gpt_generator
package gpt_generator // import "garydmenezes.com/mathgame/server/gpt_generator"

import (
	"fmt"
)

func GenerateProblem(opts *Options) (string, string, float64, error) {
	err := validateOptions(opts)
	if err != nil {
		return "", "", 0, err
	}
        // expression, answer, difficulty, error
        return "", "", 1, nil
}
