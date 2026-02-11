// Package api contains api routes, handlers, and models
package api

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Mixed number: optional minus, digits, space, digits/digits (e.g. "1 1/2", "-1 1/2")
var mixedNumberRe = regexp.MustCompile(`^(-?\d+)\s+(\d+)/(\d+)$`)

// parseAnswerToRat parses a user or stored answer string into a rational number.
// Accepts: integers (5, -3), decimals (0.5, .5, 1.5), fractions (1/2, 2/4),
// and mixed numbers (1 1/2, 2 3/4). Returns (nil, false) if s cannot be parsed.
func parseAnswerToRat(s string) (*big.Rat, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	// Normalize leading decimal point so big.Rat can parse
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	} else if strings.HasPrefix(s, "-.") {
		s = "-0" + s[1:]
	}

	// Try big.Rat.SetString first (handles integers, many decimals, and a/b fractions)
	r := new(big.Rat)
	if _, ok := r.SetString(s); ok {
		return r, true
	}

	// Mixed number: e.g. "1 1/2" -> 3/2, "-1 1/2" -> -3/2
	if m := mixedNumberRe.FindStringSubmatch(s); len(m) == 4 {
		whole, _ := strconv.Atoi(m[1])
		num, _ := strconv.Atoi(m[2])
		denom, _ := strconv.Atoi(m[3])
		if denom == 0 {
			return nil, false
		}
		// whole + num/denom = (whole*denom + num)/denom for positive whole
		// For negative whole, "N a/b" means N + a/b, so (-1) + 1/2 = -1/2
		// Common convention: "-1 1/2" = -(1 + 1/2) = -3/2
		var numer int64
		if whole >= 0 {
			numer = int64(whole)*int64(denom) + int64(num)
		} else {
			numer = int64(whole)*int64(denom) - int64(num)
		}
		denomRat := new(big.Rat).SetInt64(int64(denom))
		r.SetInt64(numer)
		r.Quo(r, denomRat)
		return r, true
	}

	// Fallback: try as float then convert to rational (e.g. "1.5", "0.25")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, false
	}
	r.SetFloat64(f)
	return r, true
}

// AnswersEquivalent reports whether userAnswer is mathematically equivalent to correctAnswer.
// E.g. 1/2 == 2/4 == 0.5 == .5, and 1.5 == 1 1/2 == 3/2.
// If either string fails to parse as a number, falls back to exact string equality.
func AnswersEquivalent(userAnswer, correctAnswer string) bool {
	if userAnswer == correctAnswer {
		return true
	}
	u, okU := parseAnswerToRat(userAnswer)
	c, okC := parseAnswerToRat(correctAnswer)
	if !okU || !okC {
		return false
	}
	return u.Cmp(c) == 0
}
