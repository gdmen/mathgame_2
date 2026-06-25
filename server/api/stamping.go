// Package api: generation-admission bookkeeping that wraps the mathcore
// pipeline.
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
// The candidate-admission stages themselves live in mathcore
// (mathcore.AdmitExpression and friends); this file holds the api-side
// orchestration: the per-call funnel, the prose-letter rewrite that keeps an
// explanation consistent with a stage-1.5 rewrite, and the exported
// answer-check wrapper used by tooling.
package api

import (
	"fmt"
	"regexp"
	"strings"

	"garydmenezes.com/mathgame/server/mathcore"
)

// Funnel stage names: every drop between "LLM returned N problems"
// and "M were inserted" must land in exactly one of these. The lexer and
// unknown-rules stages are produced by mathcore.AdmitExpression; the rest are
// orchestration-only and owned here.
const (
	rejectCollision = "collision"
	rejectAnswer    = "answer"
	rejectEnvelope  = "envelope"
	rejectValidator = "validator"
	rejectCreate    = "create"
)

// generationFunnel counts candidates through the admission pipeline for one
// generation call. Logged as a single structured line.
type generationFunnel struct {
	requested int
	returned  int
	rejects   map[string]int
	inserted  int
}

func newGenerationFunnel(requested int) *generationFunnel {
	return &generationFunnel{requested: requested, rejects: map[string]int{}}
}

func (f *generationFunnel) reject(stage string) { f.rejects[stage]++ }

// String renders the funnel as one grep-able line.
func (f *generationFunnel) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "funnel: requested=%d returned=%d", f.requested, f.returned)
	for _, stage := range []string{mathcore.RejectLexer, mathcore.RejectUnknownRules,
		rejectCollision, rejectAnswer, rejectEnvelope, rejectValidator, rejectCreate} {
		fmt.Fprintf(&b, " %s=%d", stage, f.rejects[stage])
	}
	fmt.Fprintf(&b, " inserted=%d", f.inserted)
	return b.String()
}

// RewriteLetterInProse replaces standalone occurrences of a rewritten
// variable letter in prose (explanations) with '?', keeping the explanation
// consistent with a stage-1.5-rewritten expression. Best-effort; rewritten
// rows are surfaced for spot-checking by the backfill.
func RewriteLetterInProse(s string, letter byte) string {
	if s == "" || letter == 0 {
		return s
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(string(letter)) + `\b`)
	return re.ReplaceAllString(s, "?")
}

// VerifyAnswer admits expr and checks it evaluates to answer - the exported
// form of the generation path's symbolic answer check, for tools that validate
// a candidate computation (e.g. cmd/diagnose_generation).
func VerifyAnswer(expr, answer string) error {
	adm := mathcore.AdmitExpression(expr)
	if adm.RejectStage != "" {
		return fmt.Errorf("%s: %s", adm.RejectStage, adm.RejectWhy)
	}
	return mathcore.VerifyAnswerSymbolic(adm.Tokens, answer)
}
