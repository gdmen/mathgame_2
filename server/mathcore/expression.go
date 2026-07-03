// expression.go: notation normalization, lexing, and the lone-variable rewrite.
//
// The prose rule: \text{...} contents are a single opaque prose token. Letters,
// question marks, and symbols inside \text{} can never be unknowns, operators,
// or any other structural feature. ("John has a dog." must not produce a
// variable token for the 'a'.)

package mathcore

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// TokenKind enumerates the expression alphabet. Anything outside this
// alphabet is rejected by LexExpression (blocked by default - new notation
// like ^ or \sqrt cannot enter the pool until deliberately added; see the
// new-bit checklist in docs/problem-generation.md).
type TokenKind int

const (
	TokNumber TokenKind = iota
	TokFraction
	TokOperator // one of + - * /
	TokEquals
	TokMissing // ?
	TokVariable
	TokParenOpen
	TokParenClose
	TokText // \text{...} - opaque prose
)

// Token is one lexed unit of an expression.
type Token struct {
	Kind TokenKind
	Pos  int    // byte offset in the normalized expression
	Raw  string // the exact normalized source slice

	// TokNumber
	Value          *big.Rat // numeric value; percent already divided by 100
	IsNegative     bool     // unary minus was attached
	IsDecimal      bool     // contained a decimal point
	IsPercent      bool     // had a % suffix
	DigitMagnitude float64  // |value| for integers; digit-string value for decimals (0.75 -> 75)

	// TokFraction
	Num, Den int64

	// TokOperator
	Op byte // '+', '-', '*', '/'

	// TokVariable
	Letter         byte
	HasCoefficient bool // immediately preceded by a number with no space (3x)

	// TokText
	Content string
}

// LexError reports the first unknown token encountered.
type LexError struct {
	Pos     int
	Snippet string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("unknown token at %d: %q", e.Pos, e.Snippet)
}

// Notation synonyms canonicalized by NormalizeExpression. LaTeX/unicode
// dialect forms are converted, not rejected; the lexer alphabet stays small.
// Tuned by the backfill census.
var normalizeReplacer = strings.NewReplacer(
	`\times`, "*",
	`\cdot`, "*",
	`\div`, "÷", // LaTeX division folds to the obelus; a bare / is a fraction
	`\left(`, "(",
	`\right)`, ")",
	`\dfrac`, `\frac`,
	`\tfrac`, `\frac`,
	"−", "-", // unicode minus
	"×", "*", // unicode multiplication sign
	`\$`, "", // money prefix (escaped form): $15 means the number 15
	"$", "",
	`\%`, "%", // escaped percent (display form) folds back to the literal %
)

var reFracCmd = regexp.MustCompile(`\\frac\{(\d+)\}\{(\d+)\}`)

// reFracLiteral matches an a/b fraction literal — the canonical fraction
// notation, the exact inverse of reFracCmd. A bare slash is always a fraction
// (division is the obelus ÷), so no spacing distinction is needed.
var reFracLiteral = regexp.MustCompile(`(\d+)/(\d+)`)

// reThousands joins thousands separators (15,000 -> 15000). The trailing
// group must not be a digit, so 12,3456 (garbage) is left alone; applied in
// a loop for multi-group numbers (1,234,567). RE2 has no lookahead, hence
// the captured boundary.
var reThousands = regexp.MustCompile(`(\d),(\d{3})($|[^\d])`)

// NormalizeExpression converts notation synonyms to one standard form.
// \frac{a}{b} becomes the unspaced a/b fraction convention.
// Replacements inside \text{...} are harmless (prose stays prose).
func NormalizeExpression(expr string) string {
	// Protect \text{} contents from the replacer: \times etc. inside prose is
	// vanishingly rare and converting it would only change prose cosmetically,
	// so a simple global pass is acceptable and keeps this O(n).
	s := normalizeReplacer.Replace(expr)
	s = reFracCmd.ReplaceAllString(s, "$1/$2")
	for {
		joined := reThousands.ReplaceAllString(s, "$1$2$3")
		if joined == s {
			break
		}
		s = joined
	}
	return s
}

// DisplayExpression converts a canonical (Render-produced) symbolic expression
// to valid, human-facing KaTeX: unspaced a/b → \frac{a}{b} (stacked fraction),
// ÷ → \div, * → \times, and a literal % → \% (KaTeX reads a bare % as a comment
// that eats the rest of the expression). The result is self-contained valid
// LaTeX; NormalizeExpression folds every one of these back to the grammar form,
// so it round-trips. This is a presentation skin applied at storage time,
// intended for symbolic (non-WORD) expressions.
func DisplayExpression(expr string) string {
	s := reFracLiteral.ReplaceAllString(expr, `\frac{$1}{$2}`)
	s = strings.ReplaceAll(s, " ÷ ", ` \div `)
	s = strings.ReplaceAll(s, " * ", ` \times `)
	return strings.ReplaceAll(s, "%", `\%`)
}

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isSpace(c byte) bool  { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// LexExpression tokenizes a normalized expression against the allowlist
// alphabet. Returns the first unknown token as a LexError. Division is the
// obelus ÷ and a bare / is always a fraction; the convention is documented in
// docs/problem-generation.md.
func LexExpression(expr string) ([]Token, *LexError) {
	var toks []Token
	i := 0
	n := len(expr)

	// prevMeaning classifies what came before for unary-minus and
	// coefficient decisions: 0=start, 1=operand (number/fraction/missing/
	// variable/close-paren/text), 2=operator/equals/open-paren.
	prevMeaning := 0

	for i < n {
		c := expr[i]
		switch {
		case isSpace(c):
			i++

		case strings.HasPrefix(expr[i:], `\text{`):
			start := i
			i += len(`\text{`)
			depth := 1
			for i < n && depth > 0 {
				if expr[i] == '{' {
					depth++
				} else if expr[i] == '}' {
					depth--
				}
				i++
			}
			if depth != 0 {
				return nil, &LexError{Pos: start, Snippet: snippet(expr, start)}
			}
			toks = append(toks, Token{Kind: TokText, Pos: start, Raw: expr[start:i],
				Content: expr[start+len(`\text{`) : i-1]})
			prevMeaning = 1

		case c == '(':
			toks = append(toks, Token{Kind: TokParenOpen, Pos: i, Raw: "("})
			i++
			prevMeaning = 2

		case c == ')':
			toks = append(toks, Token{Kind: TokParenClose, Pos: i, Raw: ")"})
			i++
			prevMeaning = 1

		case c == '=':
			toks = append(toks, Token{Kind: TokEquals, Pos: i, Raw: "="})
			i++
			prevMeaning = 2

		case c == '?':
			toks = append(toks, Token{Kind: TokMissing, Pos: i, Raw: "?"})
			i++
			prevMeaning = 1

		case c == '+' || c == '*':
			toks = append(toks, Token{Kind: TokOperator, Pos: i, Raw: string(c), Op: c})
			i++
			prevMeaning = 2

		case strings.HasPrefix(expr[i:], "÷"):
			// The obelus is the sole division notation; the AST op stays '/'.
			toks = append(toks, Token{Kind: TokOperator, Pos: i, Raw: "÷", Op: '/'})
			i += len("÷")
			prevMeaning = 2

		// A bare '/' is a fraction, consumed inside lexNumber (spacing-agnostic).
		// One that reaches here is not between two integers, so it is invalid.

		case c == '-':
			// Unary minus only in operand position and directly attached to a
			// digit; otherwise it is the binary subtraction operator.
			if prevMeaning != 1 && i+1 < n && isDigit(expr[i+1]) {
				start := i
				i++
				tok, lerr := lexNumber(expr, &i, start, true)
				if lerr != nil {
					return nil, lerr
				}
				toks = append(toks, tok)
				prevMeaning = 1
			} else {
				toks = append(toks, Token{Kind: TokOperator, Pos: i, Raw: "-", Op: '-'})
				i++
				prevMeaning = 2
			}

		case isDigit(c):
			start := i
			tok, lerr := lexNumber(expr, &i, start, false)
			if lerr != nil {
				return nil, lerr
			}
			// Coefficient variable: digit string immediately followed by a
			// single letter not followed by another letter (3x yes; 5km no).
			if tok.Kind == TokNumber && i < n && isLetter(expr[i]) &&
				(i+1 >= n || !isLetter(expr[i+1])) {
				// Same prose-splice guard as the standalone-letter branch:
				// "14b\text{ooks...}" is a broken prose fragment.
				crest := expr[i+1:]
				if strings.HasPrefix(crest, `\text{`) &&
					len(crest) > len(`\text{`) && !isSpace(crest[len(`\text{`)]) {
					return nil, &LexError{Pos: i, Snippet: snippet(expr, i)}
				}
				toks = append(toks, tok)
				toks = append(toks, Token{Kind: TokVariable, Pos: i, Raw: string(expr[i]),
					Letter: expr[i], HasCoefficient: true})
				i++
				prevMeaning = 1
				continue
			}
			toks = append(toks, tok)
			prevMeaning = 1

		case isLetter(c):
			// Standalone single-letter variable (not followed by a letter).
			if i+1 < n && isLetter(expr[i+1]) {
				return nil, &LexError{Pos: i, Snippet: snippet(expr, i)}
			}
			// Prose-splice guard: a letter glued to a following \text block
			// whose content continues a word ("14 b\text{ooks...}") is a
			// broken prose fragment, not a variable. Without this, the
			// stage-1.5 rewrite would splice '?' into the word ("?ooks").
			// A letter before a text block that starts with whitespace
			// ("}x\text{ apples}") is still a legitimate variable.
			rest := expr[i+1:]
			if strings.HasPrefix(rest, `\text{`) &&
				len(rest) > len(`\text{`) && !isSpace(rest[len(`\text{`)]) {
				return nil, &LexError{Pos: i, Snippet: snippet(expr, i)}
			}
			toks = append(toks, Token{Kind: TokVariable, Pos: i, Raw: string(c), Letter: c})
			i++
			prevMeaning = 1

		default:
			return nil, &LexError{Pos: i, Snippet: snippet(expr, i)}
		}
	}
	return toks, nil
}

// lexNumber consumes a number starting at *i (digits already begin there),
// handling decimals, percent suffix, and the unspaced-fraction convention.
// start is the token start (which may be one byte earlier for unary minus).
func lexNumber(expr string, i *int, start int, negative bool) (Token, *LexError) {
	n := len(expr)
	intStart := *i
	for *i < n && isDigit(expr[*i]) {
		*i++
	}
	isDecimal := false
	if *i < n && expr[*i] == '.' && *i+1 < n && isDigit(expr[*i+1]) {
		isDecimal = true
		*i++
		for *i < n && isDigit(expr[*i]) {
			*i++
		}
	}
	raw := expr[start:*i]
	numStr := expr[intStart:*i]

	// Fraction literal: <int> / <int>, spacing-agnostic. Division is the obelus
	// ÷, so a bare slash between two integers is unambiguously a fraction; the
	// integer just read is the numerator. Skip optional whitespace on either
	// side of the slash.
	if !isDecimal {
		intEnd := *i
		j := *i
		for j < n && isSpace(expr[j]) {
			j++
		}
		if j < n && expr[j] == '/' {
			j++
			for j < n && isSpace(expr[j]) {
				j++
			}
			if j < n && isDigit(expr[j]) {
				denStart := j
				for j < n && isDigit(expr[j]) {
					j++
				}
				*i = j
				nStr, dStr := expr[intStart:intEnd], expr[denStart:j]
				numV := new(big.Rat)
				if _, ok := numV.SetString(nStr + "/" + dStr); !ok {
					return Token{}, &LexError{Pos: start, Snippet: snippet(expr, start)}
				}
				var num, den big.Int
				num.SetString(nStr, 10)
				den.SetString(dStr, 10)
				// Raw is the canonical unspaced form, NOT the source slice: a
				// spaced input (`2 / 3`) must render and hash the same as `2/3`.
				raw := nStr + "/" + dStr
				if negative {
					numV.Neg(numV)
					num.Neg(&num)
					raw = "-" + raw
				}
				return Token{Kind: TokFraction, Pos: start, Raw: raw,
					Value: numV, Num: num.Int64(), Den: den.Int64(), IsNegative: negative}, nil
			}
		}
	}

	isPercent := false
	if *i < n && expr[*i] == '%' {
		isPercent = true
		*i++
		raw = expr[start:*i]
	}

	v := new(big.Rat)
	if _, ok := v.SetString(numStr); !ok {
		return Token{}, &LexError{Pos: start, Snippet: snippet(expr, start)}
	}
	if negative {
		v.Neg(v)
	}
	if isPercent {
		v.Quo(v, big.NewRat(100, 1))
	}

	// Digit magnitude: integers use |value|; decimals use the digit string
	// with the point removed (0.75 -> 75) - difficulty tracks digit
	// complexity, not numeric size.
	digits := strings.Replace(numStr, ".", "", 1)
	digits = strings.TrimLeft(digits, "0")
	var digitMag float64
	if digits != "" {
		dm := new(big.Rat)
		dm.SetString(digits)
		digitMag, _ = dm.Float64()
	}

	tok := Token{Kind: TokNumber, Pos: start, Raw: raw, Value: v,
		IsNegative: negative, IsDecimal: isDecimal, IsPercent: isPercent,
		DigitMagnitude: digitMag}
	// TokFraction also carries Value; for plain numbers Num/Den unused.
	return tok, nil
}

func snippet(s string, pos int) string {
	end := pos + 10
	if end > len(s) {
		end = len(s)
	}
	return s[pos:end]
}

// RewriteLoneVariable applies the stage-1.5 rewrite: a variable letter that
// occurs exactly once across the whole expression and has no coefficient is
// cognitively a missing-number blank, so it is rewritten to '?'
// (12 - x = 5 -> 12 - ? = 5). Letters with coefficients or multiple
// occurrences are load-bearing algebra notation and stay.
//
// Returns the (possibly modified) tokens, the rewritten expression string,
// and whether a rewrite happened. The string rewrite splices '?' at the
// variable's position so the rest of the expression is preserved verbatim.
func RewriteLoneVariable(toks []Token, expr string) ([]Token, string, bool) {
	varIdx := -1
	count := 0
	for idx, t := range toks {
		if t.Kind == TokVariable {
			count++
			varIdx = idx
		}
	}
	if count != 1 || toks[varIdx].HasCoefficient {
		return toks, expr, false
	}
	pos := toks[varIdx].Pos
	toks[varIdx] = Token{Kind: TokMissing, Pos: pos, Raw: "?"}
	rewritten := expr[:pos] + "?" + expr[pos+1:]
	return toks, rewritten, true
}

// CountDistinctUnknowns returns the number of distinct unknowns in the token
// stream: distinct variable letters, plus one if any '?' is present. Also
// returns the count of '?' occurrences. Per-problem rules:
// at most one distinct unknown, and '?' may appear at most once.
func CountDistinctUnknowns(toks []Token) (distinct int, questionMarks int) {
	letters := map[byte]bool{}
	for _, t := range toks {
		switch t.Kind {
		case TokVariable:
			letters[t.Letter] = true
		case TokMissing:
			questionMarks++
		}
	}
	distinct = len(letters)
	if questionMarks > 0 {
		distinct++
	}
	return distinct, questionMarks
}
