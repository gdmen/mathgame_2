// stamping.go: the problem admission pipeline (stages [0]-[3.5]), minus the
// final DB insert.
//
// Every candidate problem - LLM-generated or heuristic - passes through:
//
//	[0]   NORMALIZE   notation synonyms -> one standard form
//	[1]   LEX         allowlist alphabet; unknown token -> reject (+ token logged)
//	[1.5] REWRITE     lone bare variable -> ? (12 - x = 5 -> 12 - ? = 5)
//	[2]   DETECT      problem-type bits from parsed features
//	[2.5] REJECT      unknown rules: >1 distinct unknown, or ? more than once
//	[3]   VALIDATE    local-first: exact evaluator for symbolic problems
//	                  (zero LLM calls); WORD problems go to the LLM validator
//	[3.5] ENVELOPE    detected bits must be a subset of the user's bitmap
//	[4]   INSERT      problemManager.Create (caller, in package api)
//
// Disagreement = reject, not auto-correct: a generator that can't compute its
// own answer probably embedded the wrong number in the explanation prose too.

package mathcore

import (
	"fmt"
	"math/big"
	"math/bits"
	"strings"
)

// Reject-stage identifiers AdmitExpression can produce (Admission.RejectStage).
// The generation funnel in package api owns the orchestration-only stages
// (collision/answer/envelope/validator/create) and aggregates these alongside
// them.
const (
	RejectLexer        = "lexer"
	RejectUnknownRules = "unknown_rules"
)

// NormalizeProblemBitmap enforces structural invariants on a final problem
// stamp: a feature that definitionally implies another gets that bit OR'd in.
// It only ever NARROWS a problem's serving audience (never widens it), so it
// cannot leak a problem to a kid whose settings exclude it.
//
// Why it exists: the WORD-problem validator reports topic features as
// independent items and can omit one that's implied by the others (e.g.
// multiplication + subtraction without chained_operations). The parser path
// never needs this - it co-sets these from the token stream - so this is the
// deterministic backstop for the validator and legacy-preserved stamps.
// Apply at every FINAL stamp site, after any validator/legacy merge.
func NormalizeProblemBitmap(b uint64) uint64 {
	pt := ProblemType(b)
	coreOps := ADDITION | SUBTRACTION | MULTIPLICATION | DIVISION
	if bits.OnesCount64(uint64(pt&coreOps)) >= 2 {
		b |= uint64(CHAINED_OPERATIONS) // two distinct operations are two steps
	}
	if pt&PEMDAS != 0 {
		b |= uint64(CHAINED_OPERATIONS) // a precedence problem needs >=2 ops
	}
	if pt&MISMATCHED_DENOMINATORS != 0 {
		b |= uint64(FRACTIONS) // mismatched denominators require fractions
	}
	return b
}

// Admission is the result of running a candidate expression through the
// pipeline stages [0]-[2.5]. RejectStage is "" when the candidate survives.
type Admission struct {
	// Expr is the text to STORE: the original (trimmed) expression with the
	// stage-1.5 '?' splice applied when a lone letter was rewritten. The
	// original notation (\frac{1}{2}, \times) is preserved - expressions
	// render through KaTeX and normalization is a parsing concern, not a
	// storage one. Detection/difficulty normalize internally, so stamping
	// from this text is identical to stamping from the normalized form.
	Expr   string
	Tokens []Token // canonical (normalized + rewritten) tokens for the answer check
	Bitmap uint64
	// RewroteLetter is the variable letter replaced by '?' (0 if none).
	// Callers must apply the same substitution to explanation prose so the
	// kid never sees the letter the expression no longer has.
	RewroteLetter byte
	RejectStage   string
	RejectWhy     string
}

// AdmitExpression runs stages [0]-[2.5] on a raw candidate expression.
// The answer check ([3]) and envelope check ([3.5]) are separate because
// they differ for symbolic vs WORD problems. Exported for the
// recompute_problem_type_bitmap backfill, which runs the same pipeline.
func AdmitExpression(rawExpr string) Admission {
	expr := strings.TrimSpace(rawExpr)
	norm := NormalizeExpression(expr)

	toks, lexErr := LexExpression(norm)
	if lexErr != nil {
		return Admission{RejectStage: RejectLexer,
			RejectWhy: fmt.Sprintf("unknown token at %d: %q", lexErr.Pos, lexErr.Snippet)}
	}

	toks, normRewritten, rewrote := RewriteLoneVariable(toks, norm)

	distinct, qmarks := CountDistinctUnknowns(toks)
	if distinct > 1 {
		return Admission{RejectStage: RejectUnknownRules,
			RejectWhy: fmt.Sprintf("%d distinct unknowns (max 1)", distinct)}
	}
	if qmarks > 1 {
		return Admission{RejectStage: RejectUnknownRules,
			RejectWhy: fmt.Sprintf("? appears %d times (max 1)", qmarks)}
	}

	stored := expr
	var letter byte
	if rewrote {
		// Recover the spliced letter: norm and normRewritten differ at
		// exactly the splice byte.
		for i := range norm {
			if norm[i] != normRewritten[i] {
				letter = norm[i]
				break
			}
		}
		if norm == expr {
			stored = normRewritten
		} else if s, ok := spliceLoneLetterRaw(expr, letter); ok {
			stored = s
		} else {
			// Rare: dialect notation where the letter can't be located
			// unambiguously in the original text. Store the normalized
			// rewritten form - slightly denormalized display beats violating
			// the invariant that a MISSING_NUMBER-only kid never sees a
			// letter.
			stored = normRewritten
		}
	}

	return Admission{
		Expr:          stored,
		Tokens:        toks,
		Bitmap:        DetectProblemTypeBitmap(stored),
		RewroteLetter: letter,
	}
}

// spliceLoneLetterRaw replaces the single standalone occurrence of letter in
// the ORIGINAL (un-normalized) expression text with '?', skipping \text{...}
// prose. Returns ok=false unless exactly one standalone occurrence exists
// outside prose (ambiguity means the caller falls back to normalized text).
func spliceLoneLetterRaw(expr string, letter byte) (string, bool) {
	pos := -1
	count := 0
	i := 0
	n := len(expr)
	for i < n {
		if strings.HasPrefix(expr[i:], `\text{`) {
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
			continue
		}
		if expr[i] == letter {
			prevAlnum := i > 0 && (isLetter(expr[i-1]) || isDigit(expr[i-1]))
			nextAlnum := i+1 < n && (isLetter(expr[i+1]) || isDigit(expr[i+1]))
			// A coefficient form (digit-adjacent) is load-bearing and never
			// rewritten; only bare standalone occurrences count.
			if !prevAlnum && !nextAlnum {
				count++
				pos = i
			}
		}
		i++
	}
	if count != 1 {
		return "", false
	}
	return expr[:pos] + "?" + expr[pos+1:], true
}

// EnvelopeViolation returns the names of detected bits that fall outside the
// user's enabled bitmap, or "" if the problem fits the envelope.
func EnvelopeViolation(problemBits, userBitmap uint64) string {
	extra := problemBits &^ userBitmap
	if extra == 0 {
		return ""
	}
	var names []string
	for pt, name := range problemTypeNames {
		if extra&uint64(pt) != 0 {
			names = append(names, name)
		}
	}
	return strings.Join(names, ",")
}

// parseAnswerRat parses a stored answer string ("8", "3/4", "0.75", "-5")
// into an exact rational. Unicode minus is normalized first.
func parseAnswerRat(answer string) (*big.Rat, bool) {
	s := strings.TrimSpace(NormalizeExpression(answer))
	s = strings.TrimSuffix(s, "%")
	v := new(big.Rat)
	if _, ok := v.SetString(s); !ok {
		return nil, false
	}
	return v, true
}

// VerifyAnswerSymbolic checks a symbolic (non-WORD) problem's stored answer
// against the exact evaluator - the deterministic tool is authoritative here;
// zero LLM calls. Rules:
//   - the answer must parse as a rational
//   - an unknown requires an equation ('='); the answer substitutes into the
//     unknown and every side must evaluate equal
//   - with no unknown, every side must evaluate equal AND equal the answer
func VerifyAnswerSymbolic(toks []Token, answer string) error {
	ans, ok := parseAnswerRat(answer)
	if !ok {
		return fmt.Errorf("unparseable answer %q", answer)
	}

	distinct, _ := CountDistinctUnknowns(toks)
	sides := splitAtEquals(toks)
	if distinct > 0 && len(sides) < 2 {
		return fmt.Errorf("unknown present but no equation")
	}

	bind := Binding{}
	for _, t := range toks {
		switch t.Kind {
		case TokMissing:
			bind[bindingKeyMissing] = ans
		case TokVariable:
			bind[t.Letter] = ans
		}
	}

	var prev *big.Rat
	for i, side := range sides {
		v, err := EvalTokens(side, bind)
		if err != nil {
			return fmt.Errorf("side %d: %v", i+1, err)
		}
		if prev != nil && prev.Cmp(v) != 0 {
			return fmt.Errorf("equation sides disagree: %s != %s", prev, v)
		}
		prev = v
	}
	if distinct == 0 && prev.Cmp(ans) != 0 {
		return fmt.Errorf("evaluates to %s, stored answer %s", prev, ans)
	}
	return nil
}

// DetectProblemTypeBitmap inspects an expression and returns the bitmap of
// problem types it contains, mapped from the same parsed features the
// difficulty formula uses - bits, difficulty, and answers cannot disagree
// about what an expression means.
//
// The prose rule applies: structural bits (operators, unknowns, PEMDAS,
// SINGLE_VARIABLE) are token-level only; \text{...} contents never fire
// them. WORD problems' topic bits come from the validator instead. The
// magnitude bits are SHAPE bits and deliberately read prose numerals (a word
// problem about 999 apples is a LARGE_NUMBERS problem - multi-digit operands
// are visually intimidating to the audience regardless of framing), while
// DECIMALS/PERCENTAGES bits are symbolic-only.
//
// Magnitude bits are brackets, not cumulative: 13-99 stamps MEDIUM_NUMBERS,
// >= 100 stamps LARGE_NUMBERS alone ("1 + 999" is LARGE, not MEDIUM).
func DetectProblemTypeBitmap(expr string) uint64 {
	f := parseProblemFeatures(expr)
	var b ProblemType
	if f.hasAdd {
		b |= ADDITION
	}
	if f.hasSub {
		b |= SUBTRACTION
	}
	if f.hasMul {
		b |= MULTIPLICATION
	}
	if f.hasDiv {
		b |= DIVISION
	}
	if f.numFractions > 0 {
		b |= FRACTIONS
	}
	if f.hasNegatives {
		b |= NEGATIVES
	}
	if f.isWord {
		b |= WORD
	}
	if f.maxMagnitude > MediumMaxOperand {
		b |= LARGE_NUMBERS
	} else if f.maxMagnitude > SmallMaxOperand {
		b |= MEDIUM_NUMBERS
	}
	if f.numOps >= 2 {
		b |= CHAINED_OPERATIONS
	}
	if f.hasMissing {
		b |= MISSING_NUMBER
	}
	if f.numFractions >= 2 && !f.sameDenom {
		b |= MISMATCHED_DENOMINATORS
	}
	if f.hasDecimalsSymbolic {
		b |= DECIMALS
	}
	if f.requiresPEMDAS {
		b |= PEMDAS
	}
	if f.hasVariables {
		b |= SINGLE_VARIABLE
	}
	if f.hasPercentSymbolic {
		b |= PERCENTAGES
	}
	return uint64(b)
}
