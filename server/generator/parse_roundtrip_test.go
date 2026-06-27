package generator

import (
	"errors"
	"math/big"
	"math/rand"
	"testing"

	"garydmenezes.com/mathgame/server/mathcore"
)

// treeEval folds an AST directly (no re-render), the independent check that
// Parse recovered the right tree rather than a merely render-equivalent one.
func treeEval(n mathcore.Node) (*big.Rat, error) {
	switch t := n.(type) {
	case mathcore.Num:
		return new(big.Rat).Set(t.Value), nil
	case mathcore.Paren:
		return treeEval(t.X)
	case mathcore.BinaryExpr:
		l, err := treeEval(t.L)
		if err != nil {
			return nil, err
		}
		r, err := treeEval(t.R)
		if err != nil {
			return nil, err
		}
		out := new(big.Rat)
		switch t.Op {
		case '+':
			return out.Add(l, r), nil
		case '-':
			return out.Sub(l, r), nil
		case '*':
			return out.Mul(l, r), nil
		case '/':
			if r.Sign() == 0 {
				return nil, errors.New("division by zero")
			}
			return out.Quo(l, r), nil
		}
	}
	return nil, errors.New("unevaluable node")
}

// TestParseRoundTripBuilderOutputs is the round-trip property over REAL builder
// output: across every representative bitmap and integer target, the rendered
// problem parses back so that Render(Parse(s)) == s, and the parsed tree
// evaluates (treeEval) to the same value the token evaluator gives. (Structural
// equality up to associativity is covered over faithful trees in
// mathcore/parse_test.go; here the input is whatever the builder renders, whose
// trees include render-non-faithful ones.)
func TestParseRoundTripBuilderOutputs(t *testing.T) {
	rng := rand.New(rand.NewSource(20260626))
	checked, valChecked := 0, 0
	for _, bm := range representativeBitmaps() {
		ceil := mathcore.MaxDiffForBitmap(uint64(bm))
		for target := 3.0; target <= ceil; target += 1.0 {
			for k := 0; k < 4; k++ {
				expr, _, err := BuildProblem(bm, target, rng)
				if err != nil {
					t.Fatalf("BuildProblem(%d, %.0f): %v", bm, target, err)
				}
				node, perr := mathcore.Parse(expr)
				if perr != nil {
					t.Errorf("Parse(%q) error: %v (bm=%d target=%.0f)", expr, perr, bm, target)
					continue
				}
				checked++
				if rr := mathcore.Render(node); rr != expr {
					t.Errorf("not idempotent: Render(Parse(%q)) = %q", expr, rr)
				}
				// Value check on plain (non-equation) expressions: the parsed tree
				// must evaluate to the canonical token value.
				toks, lexErr := mathcore.LexExpression(mathcore.NormalizeExpression(expr))
				if lexErr != nil {
					continue
				}
				tokVal, terr := mathcore.EvalTokens(toks, mathcore.Binding{})
				treeVal, tverr := treeEval(node)
				if terr == nil && tverr == nil {
					valChecked++
					if tokVal.Cmp(treeVal) != 0 {
						t.Errorf("value mismatch for %q: tokens=%s parsedTree=%s", expr, tokVal, treeVal)
					}
				}
			}
		}
	}
	t.Logf("round-tripped %d builder expressions (%d value-checked)", checked, valChecked)
}
