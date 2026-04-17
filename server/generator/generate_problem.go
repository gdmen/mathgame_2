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

// Options configures problem generation. Backwards compatible with
// heuristic_0.0 signatures; GradeLevel added for grade-aware generation.
type Options struct {
	Operations       []string `json:"operations" form:"operations"`
	Fractions        bool     `json:"fractions" form:"fractions"`
	Negatives        bool     `json:"negatives" form:"negatives"`
	TargetDifficulty float64  `json:"target_difficulty" form:"target_difficulty"`
	GradeLevel       int      `json:"grade_level" form:"grade_level"`
}

// templateChoice picks a template probabilistically based on grade config.
// Returns the chosen template along with a short label for logging.
func pickTemplate(cfg GradeConfig, ops []Op, rng randFunc) (Template, string) {
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
		if cfg.MaxFracDenom >= 3 {
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

	cfg := getGradeConfig(opts.GradeLevel)
	ops := opsFromStrings(opts.Operations)
	if len(ops) == 0 {
		return "", "", 0, &OptionsError{s: "no valid operations"}
	}
	// Apply option-level toggles on top of grade config.
	cfg = applyOptionOverrides(cfg, opts)

	rng := rand.Intn

	// Try up to 5 times to get a valid problem from a weighted template choice.
	// If all fail, fall back to a basic binary problem.
	for attempt := 0; attempt < 5; attempt++ {
		tmpl, _ := pickTemplate(cfg, ops, rng)
		expr, ans, ok := tmpl(cfg, ops, rng)
		if ok && expr != "" && ans != "" {
			return expr, ans, opts.TargetDifficulty, nil
		}
	}
	// Last-resort fallback: basic add with small numbers.
	a := randIntRange(cfg.MinAddSub, max(cfg.MinAddSub+1, cfg.MaxAddSub))
	b := randIntRange(cfg.MinAddSub, max(cfg.MinAddSub+1, cfg.MaxAddSub))
	return formatBinary(a, OpAdd, b), fmt.Sprintf("%d", a+b), opts.TargetDifficulty, nil
}

// applyOptionOverrides lets old callers force-enable fractions/negatives
// even if the grade config wouldn't have them. Grade config still sets the
// ranges; these just flip the allow flags.
func applyOptionOverrides(cfg GradeConfig, opts *Options) GradeConfig {
	if opts.Fractions {
		cfg.AllowFrac = true
		if cfg.MaxFracDenom < 4 {
			cfg.MaxFracDenom = 8
		}
	}
	if opts.Negatives {
		cfg.AllowNeg = true
	}
	return cfg
}

func validateOptions(opts *Options) error {
	if opts == nil {
		return &OptionsError{s: "nil options"}
	}
	if len(opts.Operations) == 0 {
		return &OptionsError{s: "no operations specified"}
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
