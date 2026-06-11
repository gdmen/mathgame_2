package generator // import "garydmenezes.com/mathgame/server/generator"

import (
	"fmt"
	"math/rand"
)

// VERSION is the generator version string stamped on created problems.
// See docs/generator_versions.md for version history.
const VERSION = "heuristic_1.0"

// OptionsError is returned when options don't allow valid problem generation.
type OptionsError struct {
	s string
}

func (e *OptionsError) Error() string {
	return e.s
}

// Options configures problem generation. Bit-driven (#225): every field maps
// off the user's settings bitmap; MaxOperand is required.
type Options struct {
	Operations       []string `json:"operations" form:"operations"`
	Fractions        bool     `json:"fractions" form:"fractions"`
	Negatives        bool     `json:"negatives" form:"negatives"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`

	// MaxOperand bounds EVERY number that appears in the expression -
	// operands, fraction numerators/denominators, and values embedded by
	// missing-number templates (the sum in "? + 5 = 12"). Required (> 0).
	MaxOperand int `json:"max_operand" form:"max_operand"`
	// AllowMissing enables "? + b = c" templates (MISSING_NUMBER bit).
	AllowMissing bool `json:"allow_missing" form:"allow_missing"`
	// AllowMultiOp enables chains of 2+ operators (CHAINED_OPERATIONS bit),
	// capped at MaxChainLen operators.
	AllowMultiOp bool `json:"allow_multi_op" form:"allow_multi_op"`
	MaxChainLen  int  `json:"max_chain_len" form:"max_chain_len"`
	// SameDenomOnly restricts fraction problems to one shared denominator
	// (MISMATCHED_DENOMINATORS bit disabled).
	SameDenomOnly bool `json:"same_denom_only" form:"same_denom_only"`
}

// configFromBitOptions builds the generation config from explicit bit-driven
// options. Every numeric range respects MaxOperand
// because the insert pipeline stamps magnitude bits from every number in the
// expression: a single out-of-range embedded value would push the problem
// outside the user's envelope and waste the candidate.
func configFromBitOptions(opts *Options) GenConfig {
	maxOp := opts.MaxOperand
	capAt := func(c int) int {
		if maxOp < c {
			return maxOp
		}
		return c
	}
	chainLen := opts.MaxChainLen
	if chainLen < 2 {
		chainLen = 2
	}
	return GenConfig{
		Label:     "bitmap",
		MinAddSub: 1, MaxAddSub: maxOp,
		MinMul: 2, MaxMul: capAt(12),
		MaxDiv: maxOp, MaxDivisor: capAt(12),
		AllowFrac: opts.Fractions, MaxFracDenom: capAt(12),
		AllowNeg:      opts.Negatives,
		AllowMultiOp:  opts.AllowMultiOp,
		AllowMissing:  opts.AllowMissing,
		MaxChainLen:   chainLen,
		SameDenomOnly: opts.SameDenomOnly,
		MaxOperand:    maxOp,
	}
}

// templateChoice picks a template probabilistically based on grade config.
// Returns the chosen template along with a short label for logging.
func pickTemplate(cfg GenConfig, ops []Op, rng randFunc) (Template, string) {
	type entry struct {
		name   string
		weight int
		tmpl   Template
	}
	entries := []entry{
		{"basic", 10, tBasic},
	}
	if cfg.AllowMissing {
		entries = append(entries, entry{"missing", 3, tMissing})
	}
	if cfg.AllowMultiOp {
		entries = append(entries, entry{"multiop", 4, tMultiOp})
	}
	if cfg.AllowFrac {
		entries = append(entries, entry{"frac_same", 3, tFractionSameDenom})
		if cfg.MaxFracDenom >= 3 && !cfg.SameDenomOnly {
			entries = append(entries, entry{"frac_diff", 2, tFractionDiffDenom})
		}
	}

	total := 0
	for _, e := range entries {
		total += e.weight
	}
	r := rng(total)
	cum := 0
	for _, e := range entries {
		cum += e.weight
		if r < cum {
			return e.tmpl, e.name
		}
	}
	return entries[0].tmpl, entries[0].name
}

// GenerateProblem produces a grade-appropriate math problem.
//
// Returns (expression, answer, difficulty, error). The difficulty return
// value is kept for backward compatibility with heuristic_0.0 callers but
// is always the requested TargetDifficulty (callers pin stored difficulty
// anyway via settings.TargetDifficulty).
//
// If the grade + operations combination can't produce a valid problem after
// several attempts, returns a best-effort simple addition problem.
func GenerateProblem(opts *Options) (string, string, float64, error) {
	if err := validateOptions(opts); err != nil {
		return "", "", 0, err
	}

	cfg := configFromBitOptions(opts)
	ops := opsFromStrings(opts.Operations)
	if len(ops) == 0 {
		return "", "", 0, &OptionsError{s: "no valid operations"}
	}

	rng := rand.Intn

	// Try up to 8 times to get a valid problem from a weighted template
	// choice. The magnitude guard rejects candidates where a template
	// embedded a computed value above MaxOperand (e.g. the sum in
	// "? + 9 = 21" for a MaxOperand of 12), since the insert pipeline would
	// stamp an out-of-envelope magnitude bit on it.
	for attempt := 0; attempt < 8; attempt++ {
		tmpl, _ := pickTemplate(cfg, ops, rng)
		expr, ans, ok := tmpl(cfg, ops, rng)
		if ok && expr != "" && ans != "" && withinMaxOperand(expr, cfg.MaxOperand) {
			return expr, ans, opts.TargetDifficulty, nil
		}
	}
	// Last-resort fallback: basic add with small numbers, halved so the
	// magnitude guard can't reject it.
	hi := max(cfg.MinAddSub+1, cfg.MaxAddSub/2)
	a := randIntRange(cfg.MinAddSub, hi)
	b := randIntRange(cfg.MinAddSub, hi)
	return formatBinary(a, OpAdd, b), fmt.Sprintf("%d", a+b), opts.TargetDifficulty, nil
}

// withinMaxOperand reports whether every number appearing in the expression
// is <= maxOperand. maxOperand 0 means unbounded (legacy grade path).
func withinMaxOperand(expr string, maxOperand int) bool {
	if maxOperand <= 0 {
		return true
	}
	n := 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			if n > maxOperand {
				return false
			}
		} else {
			n = 0
		}
	}
	return true
}

func validateOptions(opts *Options) error {
	if opts == nil {
		return &OptionsError{s: "nil options"}
	}
	if len(opts.Operations) == 0 {
		return &OptionsError{s: "no operations specified"}
	}
	if opts.MaxOperand <= 0 {
		return &OptionsError{s: "MaxOperand is required (map it from the magnitude bits)"}
	}
	for _, s := range opts.Operations {
		switch s {
		case "+", "-", "*", "/",
			"add", "sub", "mul", "div",
			"addition", "subtraction", "multiplication", "division",
			"x":
			// valid
		default:
			return &OptionsError{s: fmt.Sprintf("'%s' is not a supported operation", s)}
		}
	}
	return nil
}

// max returns the larger of two ints. Local helper to avoid pulling math.Max
// on floats when both args are ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
