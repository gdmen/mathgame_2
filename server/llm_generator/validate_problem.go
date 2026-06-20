// Package llm_generator contains a math problem llm_generator
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here REQUIRE updating that doc in the same PR.
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/common"
)

// The WORD-problem validator. Called for word problems ONLY - symbolic
// problems are verified by the exact in-process evaluator (api package) with
// zero LLM calls. The validator receives the SAME MAY/MUST NOT constraints
// the generator was given, and answers:
//
//	line 1: the numeric answer (authoritative for prose math)
//	line 2: YES/NO - does the problem comply with ALL the constraints?
//	line 3: comma-separated features actually present (closed name list)
//	line 4: YES/NO - does the candidate symbolic_expression match the
//	        problem? (only when one was sent)
//
// Line 3 stamps the WORD problem's topic bits; generator self-report is
// not trusted.
const PROMPT_VALIDATION_WORD = `Given this word problem: %s
1. Solve it. Return ONLY the numeric answer on line 1 (fractions as fractions, not decimals; no LaTeX).
2. The problem was generated under these constraints:
%s
Does the problem comply with ALL of them? Answer YES or NO on line 2.
3. On line 3, list the features actually present in the problem, comma-separated, using only these names: %s.`

// PROMPT_VALIDATION_FORM is appended when a candidate symbolic_expression is
// present: it checks the form uses the operations the problem actually
// requires, catching a form that hits the answer with the wrong computation.
const PROMPT_VALIDATION_FORM = "\n4. The intended computation is %q. On line 4, answer YES only if it uses the operations and numbers this problem actually requires, NO otherwise."

// ErrEnvelopeMismatch marks a validator NO on the constraints line
// (the replacement for the old GRADE_MISMATCH).
var ErrEnvelopeMismatch = errors.New("ENVELOPE_MISMATCH")

// ErrFormMismatch marks a validator NO on the symbolic_expression line: the
// form doesn't represent the problem's actual computation.
var ErrFormMismatch = errors.New("FORM_MISMATCH")

// ValidateWordProblem checks a word problem's answer and envelope compliance
// in one LLM round-trip and extracts its observed features.
//
// featureNames is the closed list the validator may use on line 3 (the api
// side owns it). Returns the observed features on success.
func ValidateWordProblem(p *Problem, constraints string, featureNames []string) ([]string, error) {
	return ValidateWordProblemWithModel(p, constraints, featureNames, openai.GPT5)
}

// ValidateWordProblemWithModel is ValidateWordProblem with an explicit
// model, for bulk tools that trade per-call accuracy for cost.
func ValidateWordProblemWithModel(p *Problem, constraints string, featureNames []string, model string) ([]string, error) {
	if strings.ContainsAny(p.Answer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		msg := fmt.Sprintf("Answer contained text: %v\n", p)
		glog.Info(msg)
		return nil, errors.New(msg)
	}

	c, err := common.ReadConfig("conf.json")
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	prompt := fmt.Sprintf(PROMPT_VALIDATION_WORD,
		p.Expression, constraints, strings.Join(featureNames, ", "))
	wantLines := 3
	if p.SymbolicExpression != "" {
		prompt += fmt.Sprintf(PROMPT_VALIDATION_FORM, p.SymbolicExpression)
		wantLines = 4
	}
	prompt += fmt.Sprintf("\nReturn exactly %d lines and nothing else.", wantLines)
	glog.Infof("OpenAI validation prompt = expected answer: %s = %s\n", prompt, p.Answer)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := chatCompletionWithRetry(
		context.Background(),
		client,
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)
	if err != nil {
		glog.Infof("OpenAI error when validating (after retries): %v\n", err)
		return nil, err
	}

	features, err := parseValidatorResponse(resp.Choices[0].Message.Content, p)
	if err != nil {
		glog.Infof("validator reject: %v (%q)", err, p.Expression)
	}
	return features, err
}

// parseValidatorResponse interprets the validator's lines: answer (1), envelope
// YES/NO (2), features (3), and - when a symbolic_expression was sent -
// form-matches-problem YES/NO (4). Returns the observed features, or an error
// for an answer mismatch / envelope NO / form NO.
func parseValidatorResponse(content string, p *Problem) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("validator returned %d lines: %q", len(lines), content)
	}

	if answer := strings.TrimSpace(lines[0]); answer != p.Answer {
		return nil, fmt.Errorf("validator answer %q, expected %q", answer, p.Answer)
	}

	if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(lines[1])), "YES") {
		return nil, ErrEnvelopeMismatch
	}

	var features []string
	if len(lines) >= 3 {
		for _, f := range strings.Split(lines[2], ",") {
			if f = strings.ToLower(strings.TrimSpace(f)); f != "" {
				features = append(features, f)
			}
		}
	}

	if p.SymbolicExpression != "" && len(lines) >= 4 {
		if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(lines[3])), "YES") {
			return nil, ErrFormMismatch
		}
	}
	return features, nil
}
