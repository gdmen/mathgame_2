// evaluator.go: exact rational evaluation over lexer tokens.
//
// Two evaluations share one token stream:
//   - EvalTokens: the CORRECT evaluation - parses the tokens into the AST
//     (Parse) and folds the tree (Eval), honoring parentheses and precedence.
//   - EvalTokensNaiveLTR: the NAIVE evaluation - parentheses stripped, a
//     single strict left-to-right fold.
//
// PEMDAS (requires non-left-to-right evaluation) fires iff the two disagree.
// All arithmetic is math/big.Rat - exact rationals, no float epsilon, no LLM,
// no eval() of untrusted input. Because the answer check, the PEMDAS
// dual-eval, and bit detection all consume the same tokens, difficulty, bits,
// and answers cannot disagree about what an expression means.

package mathcore

import (
	"errors"
	"math/big"
)

var (
	errEvalEmpty     = errors.New("eval: empty expression side")
	errEvalMalformed = errors.New("eval: malformed expression")
	errEvalDivZero   = errors.New("eval: division by zero")
	errEvalUnbound   = errors.New("eval: unbound unknown")
)

// Binding resolves unknown tokens (TokMissing / TokVariable) to values.
// A nil map means unknowns are errors.
type Binding map[byte]*big.Rat

// bindingKeyMissing is the Binding key for the '?' unknown.
const bindingKeyMissing byte = '?'

func resolveOperand(t Token, bind Binding) (*big.Rat, error) {
	switch t.Kind {
	case TokNumber, TokFraction:
		return t.Value, nil
	case TokMissing:
		if v, ok := bind[bindingKeyMissing]; ok {
			return v, nil
		}
		return nil, errEvalUnbound
	case TokVariable:
		if v, ok := bind[t.Letter]; ok {
			return v, nil
		}
		return nil, errEvalUnbound
	default:
		return nil, errEvalMalformed
	}
}

func applyOp(op byte, a, b *big.Rat) (*big.Rat, error) {
	out := new(big.Rat)
	switch op {
	case '+':
		return out.Add(a, b), nil
	case '-':
		return out.Sub(a, b), nil
	case '*':
		return out.Mul(a, b), nil
	case '/':
		if b.Sign() == 0 {
			return nil, errEvalDivZero
		}
		return out.Quo(a, b), nil
	default:
		return nil, errEvalMalformed
	}
}

// EvalTokens evaluates one expression side by parsing it into the AST (Parse)
// and folding the tree (Eval). Precedence/associativity live only in Parse, so
// there is one grammar implementation, not a second recursive descent here.
func EvalTokens(toks []Token, bind Binding) (*big.Rat, error) {
	node, err := ParseTokens(toks)
	if err != nil {
		if err == errParseEmpty {
			return nil, errEvalEmpty
		}
		return nil, errEvalMalformed
	}
	return Eval(node, bind)
}

// Eval folds an AST node to an exact rational, resolving TokMissing/TokVariable
// unknowns through bind (nil binds nothing, so an unknown errors). A coefficient
// term is the BinaryExpr{'*', Num, Var} the parser builds, so it folds like any
// other product. An Equation has no single value and errors.
func Eval(n Node, bind Binding) (*big.Rat, error) {
	switch t := n.(type) {
	case Num:
		// Copy: callers must never alias (and risk mutating) a node's Value.
		return new(big.Rat).Set(t.Value), nil
	case Missing:
		if v, ok := bind[bindingKeyMissing]; ok {
			return v, nil
		}
		return nil, errEvalUnbound
	case Var:
		if v, ok := bind[t.Letter]; ok {
			return v, nil
		}
		return nil, errEvalUnbound
	case Paren:
		return Eval(t.X, bind)
	case BinaryExpr:
		l, err := Eval(t.L, bind)
		if err != nil {
			return nil, err
		}
		r, err := Eval(t.R, bind)
		if err != nil {
			return nil, err
		}
		return applyOp(t.Op, l, r)
	}
	return nil, errEvalMalformed
}

// EvalTokensNaiveLTR evaluates one expression side as a naive reader with no
// precedence knowledge would: parentheses are stripped entirely and the
// remaining operand/operator sequence is folded strictly left to right.
// A coefficient pair (3x) reads as one operand even to a naive reader.
func EvalTokensNaiveLTR(toks []Token, bind Binding) (*big.Rat, error) {
	var flat []Token
	for _, t := range toks {
		if t.Kind == TokParenOpen || t.Kind == TokParenClose {
			continue
		}
		flat = append(flat, t)
	}
	if len(flat) == 0 {
		return nil, errEvalEmpty
	}
	// readOperand consumes one operand at i, merging coefficient pairs.
	readOperand := func(i int) (*big.Rat, int, error) {
		v, err := resolveOperand(flat[i], bind)
		if err != nil {
			return nil, i, err
		}
		i++
		if flat[i-1].Kind == TokNumber && i < len(flat) &&
			flat[i].Kind == TokVariable && flat[i].HasCoefficient {
			vv, err := resolveOperand(flat[i], bind)
			if err != nil {
				return nil, i, err
			}
			v = new(big.Rat).Mul(v, vv)
			i++
		}
		return v, i, nil
	}

	acc, i, err := readOperand(0)
	if err != nil {
		return nil, err
	}
	for i < len(flat) {
		if flat[i].Kind != TokOperator || i+1 >= len(flat) {
			return nil, errEvalMalformed
		}
		op := flat[i].Op
		right, next, err := readOperand(i + 1)
		if err != nil {
			return nil, err
		}
		acc, err = applyOp(op, acc, right)
		if err != nil {
			return nil, err
		}
		i = next
	}
	return acc, nil
}

// pemdasProbes are fixed rational probe values substituted for unknowns when
// deciding whether an expression requires non-left-to-right evaluation. The
// formula must stay a pure function of the expression (the recompute
// machinery depends on determinism), so the stored answer is NOT consulted;
// instead, if the two evaluation orders disagree as symbolic trees they
// disagree for all but finitely many values - two independent probes make a
// coincidental agreement on both vanishingly unlikely and deterministic.
var pemdasProbes = []*big.Rat{
	big.NewRat(1009, 7),
	big.NewRat(104729, 337),
}

// splitAtEquals splits a token stream into equation sides (1 side if no '=').
func splitAtEquals(toks []Token) [][]Token {
	var sides [][]Token
	start := 0
	for i, t := range toks {
		if t.Kind == TokEquals {
			sides = append(sides, toks[start:i])
			start = i + 1
		}
	}
	sides = append(sides, toks[start:])
	return sides
}

// requiresPEMDAS reports whether any equation side of the token stream
// evaluates differently under correct vs naive left-to-right order - i.e.
// whether solving the problem REQUIRES knowing precedence/parens.
//
//	2 * 3 + 5    LTR 11 = correct 11  -> false
//	5 + 2 * 3    LTR 21 != correct 11 -> true
//	(3 + 5) * 2  LTR 16 = correct 16  -> false (parens spell out natural order)
//	12 - (5 - 3) LTR 4 != correct 10  -> true (no multiplication needed)
//
// Naive division-by-zero where correct succeeds counts as disagreement
// (the orders disagree in the strongest sense). If the CORRECT evaluation
// errors the problem is malformed and will be rejected elsewhere; no fire.
// Unknowns are bound to fixed probes (see pemdasProbes).
func requiresPEMDAS(toks []Token) bool {
	for _, rawSide := range splitAtEquals(toks) {
		// Prose tokens are labels between math, not operands: filter them out
		// so a mixed-form word problem (\text{Solve: }5 + 2 * 3) is judged on
		// its symbolic part. A side whose math is fragmented by prose into a
		// malformed sequence simply fails evaluation below and never fires.
		var side []Token
		for _, t := range rawSide {
			if t.Kind != TokText {
				side = append(side, t)
			}
		}
		ops := 0
		hasUnknown := false
		for _, t := range side {
			if t.Kind == TokOperator {
				ops++
			}
			if t.Kind == TokMissing || t.Kind == TokVariable {
				hasUnknown = true
			}
		}
		if ops < 2 {
			continue // a single op cannot disagree with itself
		}
		probes := pemdasProbes[:1]
		if hasUnknown {
			probes = pemdasProbes
		}
		for _, p := range probes {
			bind := Binding{}
			for _, t := range side {
				if t.Kind == TokMissing {
					bind[bindingKeyMissing] = p
				} else if t.Kind == TokVariable {
					bind[t.Letter] = p
				}
			}
			correct, errC := EvalTokens(side, bind)
			if errC != nil {
				continue // malformed or correct-side div0: rejected elsewhere
			}
			naive, errN := EvalTokensNaiveLTR(side, bind)
			if errN != nil {
				return true // naive fails where correct succeeds: disagreement
			}
			if correct.Cmp(naive) != 0 {
				return true
			}
		}
	}
	return false
}
