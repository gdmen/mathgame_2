// Package gpt_generator contains a math problem gpt_generator
package gpt_generator // import "garydmenezes.com/mathgame/server/gpt_generator"

import (
	"fmt"
)

type Operation struct {
	getInputDiff GetInputDiff
	do           Operate
	String       func() string
}

var operations = map[string]Operation{
	"+": {
		getInputDiff: addInputDiff,
		do:           add,
		String:       func() string { return "add" },
	},
	"-": {
		getInputDiff: subInputDiff,
		do:           sub,
		String:       func() string { return "sub" },
	},
	"*": {
		getInputDiff: mulInputDiff,
		do:           mul,
		String:       func() string { return "mul" },
	},
}

func isSupportedOperation(o string) bool {
	_, ok := operations[o]
	return ok
}
