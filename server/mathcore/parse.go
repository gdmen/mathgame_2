// parse.go: the structural inverse of Render — token stream -> AST.
//
// Parse mirrors EvalTokens' grammar exactly (parens, then * and /, then + and
// -, left-associative within a precedence level), so for any canonical
// (Render-produced) expression s: Render(Parse(s)) == s, and the parsed tree
// evaluates to EvalTokens(s). The slash fraction/division convention it relies
// on is documented in docs/problem-generation.md.

package mathcore

import (
	"errors"
	"math/big"
)

var (
	errParseEmpty    = errors.New("parse: empty token stream")
	errParseTrailing = errors.New("parse: unconsumed trailing tokens")
	errParseOperand  = errors.New("parse: expected an operand")
	errParseParen    = errors.New("parse: unbalanced parenthesis")
	errParseText     = errors.New(`parse: \text is prose, not part of the symbolic AST`)
)

// Parse normalizes, lexes, and parses a symbolic expression into the AST. WORD
// problems (\text{...}) are not representable in the AST and are rejected.
func Parse(expr string) (Node, error) {
	toks, lexErr := LexExpression(NormalizeExpression(expr))
	if lexErr != nil {
		return nil, lexErr
	}
	return ParseTokens(toks)
}

// ParseTokens parses an already-lexed token stream into the AST.
func ParseTokens(toks []Token) (Node, error) {
	if len(toks) == 0 {
		return nil, errParseEmpty
	}
	p := &parser{toks: toks}
	node, err := p.equationOrExpr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, errParseTrailing
	}
	return node, nil
}

type parser struct {
	toks []Token
	pos  int
}

func (p *parser) peek() (Token, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return Token{}, false
}

// equationOrExpr := expr ('=' expr)?
func (p *parser) equationOrExpr() (Node, error) {
	lhs, err := p.expr()
	if err != nil {
		return nil, err
	}
	if t, ok := p.peek(); ok && t.Kind == TokEquals {
		p.pos++
		rhs, err := p.expr()
		if err != nil {
			return nil, err
		}
		return Equation{LHS: lhs, RHS: rhs}, nil
	}
	return lhs, nil
}

// expr := term (('+'|'-') term)*  — left-associative.
func (p *parser) expr() (Node, error) {
	left, err := p.term()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.Kind != TokOperator || (t.Op != '+' && t.Op != '-') {
			return left, nil
		}
		p.pos++
		right, err := p.term()
		if err != nil {
			return nil, err
		}
		left = BinaryExpr{Op: t.Op, L: left, R: right}
	}
}

// term := factor (('*'|'/') factor)*  — left-associative.
func (p *parser) term() (Node, error) {
	left, err := p.factor()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.Kind != TokOperator || (t.Op != '*' && t.Op != '/') {
			return left, nil
		}
		p.pos++
		right, err := p.factor()
		if err != nil {
			return nil, err
		}
		left = BinaryExpr{Op: t.Op, L: left, R: right}
	}
}

// factor := '(' expr ')' | operand
func (p *parser) factor() (Node, error) {
	t, ok := p.peek()
	if !ok {
		return nil, errParseOperand
	}
	if t.Kind == TokParenOpen {
		p.pos++
		inner, err := p.expr()
		if err != nil {
			return nil, err
		}
		if c, ok := p.peek(); !ok || c.Kind != TokParenClose {
			return nil, errParseParen
		}
		p.pos++
		return Paren{X: inner}, nil
	}
	return p.operand()
}

// operand := NUMBER (coefficient-VARIABLE)? | FRACTION | MISSING | VARIABLE
func (p *parser) operand() (Node, error) {
	t, ok := p.peek()
	if !ok {
		return nil, errParseOperand
	}
	switch t.Kind {
	case TokNumber:
		p.pos++
		num := Num{Value: new(big.Rat).Set(t.Value), Raw: t.Raw, IsDecimal: t.IsDecimal, IsPercent: t.IsPercent}
		// A coefficient variable glued to this number (3x) is one operand,
		// modeled as the same BinaryExpr{'*', Num, Var} the builder/Render use.
		if nx, ok := p.peek(); ok && nx.Kind == TokVariable && nx.HasCoefficient {
			p.pos++
			return BinaryExpr{Op: '*', L: num, R: Var{Letter: nx.Letter, HasCoefficient: true}}, nil
		}
		return num, nil
	case TokFraction:
		p.pos++
		return Num{Value: new(big.Rat).Set(t.Value), Raw: t.Raw, IsFraction: true}, nil
	case TokMissing:
		p.pos++
		return Missing{}, nil
	case TokVariable:
		p.pos++
		return Var{Letter: t.Letter, HasCoefficient: t.HasCoefficient}, nil
	case TokText:
		return nil, errParseText
	default:
		return nil, errParseOperand
	}
}
