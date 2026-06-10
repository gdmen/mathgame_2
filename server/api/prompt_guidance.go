// Package api: MAY / MUST NOT prompt constraints derived from a settings
// bitmap.
//
// Part of the problem-generation system - documented in docs/problem-generation.md
// (lands with issue #225 PR3). Behavior changes here REQUIRE updating that doc
// in the same PR.
//
// Semantics: an enabled bit means the generator MAY include that feature
// (allows, doesn't require); a disabled bit means it MUST NOT (hard
// prohibition). Every constraint the insert pipeline enforces must also be
// communicated here - otherwise the generator wastes output on shapes that
// always reject (the #110 failure mode).
package api

import (
	"fmt"
	"strings"
)

// bitGuidance holds the May/MustNot sentence pair per bit. Magnitude and
// chain bits are expressed as dedicated multi-state clauses below instead.
type bitGuidance struct {
	may     string
	mustNot string
}

// promptGuidanceOrder fixes the emission order (stable prompts are easier to
// debug and cache).
var promptGuidanceOrder = []ProblemType{
	ADDITION, SUBTRACTION, MULTIPLICATION, DIVISION,
	FRACTIONS, MISMATCHED_DENOMINATORS, DECIMALS, PERCENTAGES, NEGATIVES,
	WORD, MISSING_NUMBER, SINGLE_VARIABLE, PEMDAS,
}

var promptGuidance = map[ProblemType]bitGuidance{
	ADDITION:       {"use addition", "use addition"},
	SUBTRACTION:    {"use subtraction", "use subtraction"},
	MULTIPLICATION: {"use multiplication", "use multiplication"},
	DIVISION:       {"use division", "use division"},
	FRACTIONS:      {"include fractions (written unspaced, e.g. 3/8)", "include any fractions"},
	MISMATCHED_DENOMINATORS: {
		"use fractions with different denominators in the same problem",
		"mix denominators: every fraction within a single problem MUST share one denominator",
	},
	DECIMALS:    {"use decimal numbers (e.g. 0.75)", "use decimal numbers"},
	PERCENTAGES: {"use percentages (e.g. 25%)", "use percentages"},
	NEGATIVES:   {"use negative numbers", "use negative numbers or produce negative answers"},
	WORD: {
		"pose word problems: prose inside \\text{...}, symbolic math outside the \\text{} blocks",
		"include any prose or word problems - bare symbolic expressions only",
	},
	MISSING_NUMBER: {
		"pose fill-in-the-blank equations using ? for the blank (e.g. ? + 5 = 12)",
		"use ? blanks or fill-in-the-blank equations",
	},
	SINGLE_VARIABLE: {
		"use a single variable letter that has a coefficient or appears more than once (e.g. 3x + 7 = 22)",
		"use variable letters",
	},
	PEMDAS: {
		"pose expressions whose result depends on operator precedence or parentheses (e.g. 5 + 2 * 3)",
		"pose expressions whose result depends on operator precedence or parentheses - expressions must evaluate the same left-to-right",
	},
}

// unknownRulesClause is emitted whenever MISSING_NUMBER or SINGLE_VARIABLE is
// enabled. It mirrors the insert pipeline's per-problem unknown rules exactly
// (at most one distinct unknown; '?' at most once).
const unknownRulesClause = "Each problem may contain at most ONE unknown - either a single missing-number placeholder (?) or a single variable letter. A ? may appear at most once; if the unknown must appear multiple times, use a variable letter. Two different unknowns may never appear in one problem."

// closedWorldClause is always emitted: anything not explicitly allowed is
// forbidden, which is what keeps the lexer reject-rate low.
const closedWorldClause = "Use ONLY the operations and concepts explicitly allowed above. Do not introduce any other mathematical notation or concepts (no square roots, exponents, modulo, absolute value, factorials, etc.)."

// BuildBitConstraints renders the full MAY / MUST NOT constraint block for a
// settings bitmap. Used by both the generation prompt and the WORD-problem
// validator (the validator judges compliance against the SAME constraints the
// generator was given).
func BuildBitConstraints(enabled ProblemType) string {
	var b strings.Builder
	b.WriteString("Constraints - ALL are simultaneous:\n")

	for _, bit := range promptGuidanceOrder {
		g := promptGuidance[bit]
		if enabled&bit != 0 {
			fmt.Fprintf(&b, "- MAY %s.\n", g.may)
		} else {
			// MISMATCHED's MustNot only makes sense when fractions exist at all.
			if bit == MISMATCHED_DENOMINATORS && enabled&FRACTIONS == 0 {
				continue
			}
			fmt.Fprintf(&b, "- MUST NOT %s.\n", g.mustNot)
		}
	}

	// 3-state magnitude clause. Digit complexity, universally: a decimal's
	// digit string counts (0.75 counts as 75).
	switch {
	case enabled&LARGE_NUMBERS != 0:
		fmt.Fprintf(&b, "- Numbers MAY be any size up to %d. A decimal's digits count as its size (0.75 counts as 75).\n", LargeMaxOperand)
	case enabled&MEDIUM_NUMBERS != 0:
		b.WriteString("- Numbers MAY be 1-99 and MUST NOT exceed 99. A decimal's digits count as its size (0.75 counts as 75).\n")
	default:
		b.WriteString("- Every number MUST be between 1 and 12. A decimal's digits count as its size (0.75 counts as 75, which exceeds 12).\n")
	}

	// 2-state chain clause.
	if enabled&CHAINED_OPERATIONS != 0 {
		fmt.Fprintf(&b, "- Expressions MAY chain 2 or more operators (at most %d); single-operator problems are still allowed.\n", MaxChainLen)
	} else {
		b.WriteString("- Every expression MUST have exactly one operator.\n")
	}

	if enabled&(MISSING_NUMBER|SINGLE_VARIABLE) != 0 {
		b.WriteString("- " + unknownRulesClause + "\n")
	}

	b.WriteString("- " + closedWorldClause + "\n")
	b.WriteString("All constraints above apply to every problem simultaneously.")
	return b.String()
}

// ValidatorFeatureNames is the closed list of feature names the WORD-problem
// validator may report on its features line. Only these names are mapped to
// bits when stamping validator-extracted topics (FeaturesToProblemType
// ignores unknown names, but the prompt should not invite free-form output).
var ValidatorFeatureNames = []string{
	"addition", "subtraction", "multiplication", "division",
	"fractions", "negatives", "decimals", "percentages", "word",
}
