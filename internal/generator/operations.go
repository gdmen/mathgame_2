// Package generator contains a math problem generator
package generator // import "garydmenezes.com/mathgame/internal/generator"

import (
	"fmt"
)

const (
	addDiffMultiplier = 1.0
	subDiffMultiplier = 1.2
)

type Operation struct {
	getInputDiff GetInputDiff
	do           Operate
	String       func() string
}

type GetInputDiff func(maxDiff float64) (maxInputDiff float64)

type Operate func(a *Problem, b *Problem, opts *Options) (prob *Problem)

func addInputDiff(maxDiff float64) float64 {
	return maxDiff / addDiffMultiplier
}

func add(a *Problem, b *Problem, opts *Options) *Problem {
	prob := &Problem{}
	prob.Expr = fmt.Sprintf("%s+%s", a.Expr, b.Expr)
	a_v, b_v := a.GetAns(), b.GetAns()
	prob.SetAns(a_v.Add(a_v, b_v))
	prob.Diff = (a.Diff + b.Diff) * addDiffMultiplier
	return prob
}

func subInputDiff(maxDiff float64) float64 {
	return maxDiff / subDiffMultiplier
}

func sub(a *Problem, b *Problem, opts *Options) *Problem {
	prob := &Problem{}
	if !opts.Negatives {
		a, b = SortProblems(a, b)
	}
	expr_fmt := "%s-(%s)"
	if b.isNumber {
		expr_fmt = "%s-%s"
	}
	prob.Expr = fmt.Sprintf(expr_fmt, a.Expr, b.Expr)
	a_v, b_v := a.GetAns(), b.GetAns()
	prob.SetAns(a_v.Sub(a_v, b_v))
	prob.Diff = (a.Diff + b.Diff) * subDiffMultiplier
	return prob
}

var operations = map[string]Operation{
	"+": Operation{
		getInputDiff: addInputDiff,
		do:           add,
		String:       func() string { return "add" },
	},
	"-": Operation{
		getInputDiff: subInputDiff,
		do:           sub,
		String:       func() string { return "sub" },
	},
}

func isSupportedOperation(o string) bool {
	_, ok := operations[o]
	return ok
}
