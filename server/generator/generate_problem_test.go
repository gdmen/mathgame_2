package generator

import (
	"strconv"
	"strings"
	"testing"
)

// evalSimple evaluates a left-to-right binary or chain expression of integers
// ignoring precedence. Used only to verify test-generated problems.
// Supports "a op b" and "a op b op c ..." with spaces.
// Does NOT handle fractions or missing-number templates.
func evalSimple(t *testing.T, expr string) int {
	t.Helper()
	parts := strings.Fields(expr)
	if len(parts) < 3 {
		t.Fatalf("can't eval %q: need at least 3 tokens", expr)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("can't parse %q: %v", parts[0], err)
	}
	for i := 1; i < len(parts); i += 2 {
		op := parts[i]
		val, err := strconv.Atoi(parts[i+1])
		if err != nil {
			t.Fatalf("can't parse %q: %v", parts[i+1], err)
		}
		switch op {
		case "+":
			n += val
		case "-":
			n -= val
		case "*":
			n *= val
		case "/":
			if val == 0 {
				t.Fatalf("division by zero in %q", expr)
			}
			if n%val != 0 {
				t.Fatalf("non-integer division in %q: %d/%d", expr, n, val)
			}
			n /= val
		default:
			t.Fatalf("unknown op %q in %q", op, expr)
		}
	}
	return n
}

// TestGenerateProblem_Grade1 verifies grade 1 produces add/sub within [1,20]
// with non-trivial operands.
func TestGenerateProblem_Grade1(t *testing.T) {
	opts := &Options{
		Operations: []string{"+", "-"},
		GradeLevel: 1,
	}
	for i := 0; i < 100; i++ {
		expr, ans, _, err := GenerateProblem(opts)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// Simple binary problems are easy to verify
		if !strings.Contains(expr, "=") {
			got := evalSimple(t, expr)
			want, _ := strconv.Atoi(ans)
			if got != want {
				t.Errorf("expr=%q got=%d want=%s", expr, got, ans)
			}
			// verify operands are in grade 1 range [1, 20]
			for _, p := range strings.Fields(expr) {
				if n, err := strconv.Atoi(p); err == nil {
					if n > 20 || n < -20 {
						t.Errorf("grade 1 operand out of range: %d in %q", n, expr)
					}
				}
			}
		}
	}
}

// TestGenerateProblem_Grade5 verifies grade 5 can produce fractions.
func TestGenerateProblem_Grade5(t *testing.T) {
	opts := &Options{
		Operations: []string{"+", "-", "*", "/"},
		GradeLevel: 5,
	}
	sawFraction := false
	sawMul := false
	for i := 0; i < 200; i++ {
		expr, _, _, err := GenerateProblem(opts)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// fraction detection: "/" without surrounding spaces
		if strings.Count(expr, "/") > strings.Count(expr, " / ") {
			sawFraction = true
		}
		if strings.Contains(expr, " * ") {
			sawMul = true
		}
	}
	if !sawFraction {
		t.Error("grade 5 with fractions enabled never produced a fraction in 200 tries")
	}
	if !sawMul {
		t.Error("grade 5 with multiplication enabled never produced a multiplication in 200 tries")
	}
}

// TestGenerateProblem_CleanFormatting verifies expressions don't have
// legacy-style paren wrapping like "(3)+(5)".
func TestGenerateProblem_CleanFormatting(t *testing.T) {
	opts := &Options{
		Operations: []string{"+", "-", "*", "/"},
		GradeLevel: 4,
	}
	for i := 0; i < 100; i++ {
		expr, _, _, err := GenerateProblem(opts)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if strings.Contains(expr, ")(") {
			t.Errorf("expression has juxtaposed parens (legacy format): %q", expr)
		}
		// Simple numbers wrapped in parens like "(3)" would be legacy style.
		// Check for "(digit" patterns.
		for _, c := range []string{"(0", "(1", "(2", "(3", "(4", "(5", "(6", "(7", "(8", "(9"} {
			if strings.Contains(expr, c) {
				t.Errorf("expression wraps single number in parens: %q (found %q)", expr, c)
				break
			}
		}
	}
}

// TestGenerateProblem_DivisionAlwaysClean verifies division problems always
// have integer results (no remainders).
func TestGenerateProblem_DivisionAlwaysClean(t *testing.T) {
	opts := &Options{
		Operations: []string{"/"},
		GradeLevel: 4,
	}
	for i := 0; i < 100; i++ {
		expr, ans, _, err := GenerateProblem(opts)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if strings.Contains(expr, "=") {
			continue // missing-number templates
		}
		// Simple "a / b" form
		parts := strings.Fields(expr)
		if len(parts) == 3 && parts[1] == "/" {
			a, _ := strconv.Atoi(parts[0])
			b, _ := strconv.Atoi(parts[2])
			if b == 0 {
				t.Errorf("division by zero: %q", expr)
				continue
			}
			if a%b != 0 {
				t.Errorf("division has remainder: %q (a=%d, b=%d)", expr, a, b)
			}
			want := a / b
			got, _ := strconv.Atoi(ans)
			if got != want {
				t.Errorf("wrong answer: %q = %d, got %s", expr, want, ans)
			}
		}
	}
}

// TestGenerateProblem_MissingNumber verifies missing-number templates when
// grade config allows them.
func TestGenerateProblem_MissingNumber(t *testing.T) {
	opts := &Options{
		Operations: []string{"+", "-"},
		GradeLevel: 3,
	}
	sawMissing := false
	for i := 0; i < 200; i++ {
		expr, _, _, err := GenerateProblem(opts)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if strings.Contains(expr, "?") {
			sawMissing = true
			if !strings.Contains(expr, "=") {
				t.Errorf("missing-number template should have '=': %q", expr)
			}
		}
	}
	if !sawMissing {
		t.Error("grade 3 with AllowMissing never produced a missing-number problem in 200 tries")
	}
}

// TestGenerateProblem_Version verifies the VERSION constant is correct.
func TestGenerateProblem_Version(t *testing.T) {
	if VERSION != "heuristic_1.0" {
		t.Errorf("expected VERSION=heuristic_1.0, got %q", VERSION)
	}
}

// TestGenerateProblem_NoEmptyOrInvalid ensures the generator never returns
// empty expressions or empty answers under valid options.
func TestGenerateProblem_NoEmptyOrInvalid(t *testing.T) {
	cases := []struct {
		name string
		opts *Options
	}{
		{"grade1 +/-", &Options{Operations: []string{"+", "-"}, GradeLevel: 1}},
		{"grade2 +/-/*", &Options{Operations: []string{"+", "-", "*"}, GradeLevel: 2}},
		{"grade3 all", &Options{Operations: []string{"+", "-", "*", "/"}, GradeLevel: 3}},
		{"grade8 all", &Options{Operations: []string{"+", "-", "*", "/"}, GradeLevel: 8}},
		{"no grade", &Options{Operations: []string{"+", "-"}, GradeLevel: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 50; i++ {
				expr, ans, _, err := GenerateProblem(tc.opts)
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if expr == "" {
					t.Errorf("empty expression")
				}
				if ans == "" {
					t.Errorf("empty answer")
				}
			}
		})
	}
}

// TestGenerateProblem_InvalidOptions checks error cases.
func TestGenerateProblem_InvalidOptions(t *testing.T) {
	_, _, _, err := GenerateProblem(&Options{})
	if err == nil {
		t.Error("expected error for empty operations")
	}
	_, _, _, err = GenerateProblem(&Options{Operations: []string{"bogus"}})
	if err == nil {
		t.Error("expected error for invalid operation")
	}
}

// TestGenerateProblem_NoTrivialProblems verifies the generator doesn't produce
// problems like "5 + 0", "7 - 7", "5 * 1" that are trivial.
// Note: This is a statistical test - we accept some trivial problems but
// they should be rare.
func TestGenerateProblem_NoTrivialProblems(t *testing.T) {
	opts := &Options{
		Operations: []string{"+", "-", "*"},
		GradeLevel: 3,
	}
	trivialCount := 0
	total := 500
	for i := 0; i < total; i++ {
		expr, _, _, err := GenerateProblem(opts)
		if err != nil {
			continue
		}
		parts := strings.Fields(expr)
		if len(parts) != 3 {
			continue // skip multi-term and missing-number
		}
		a, aerr := strconv.Atoi(parts[0])
		b, berr := strconv.Atoi(parts[2])
		if aerr != nil || berr != nil {
			continue
		}
		// Check for trivial patterns
		if a == 0 || b == 0 {
			trivialCount++
		}
		if parts[1] == "*" && (a == 1 || b == 1) {
			trivialCount++
		}
		if parts[1] == "-" && a == b {
			trivialCount++
		}
	}
	if trivialCount > total/20 { // allow up to 5% trivial
		t.Errorf("too many trivial problems: %d/%d (%.1f%%)",
			trivialCount, total, 100*float64(trivialCount)/float64(total))
	}
}
