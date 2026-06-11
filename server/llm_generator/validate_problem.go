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
// the generator was given, and answers three lines:
//
//	line 1: the numeric answer (authoritative for prose math)
//	line 2: YES/NO - does the problem comply with ALL the constraints?
//	line 3: comma-separated features actually present (closed name list)
//
// Line 3 stamps the WORD problem's topic bits; generator self-report is
// not trusted.
const PROMPT_VALIDATION_WORD = `Given this word problem: %s
1. Solve it. Return ONLY the numeric answer on line 1 (fractions as fractions, not decimals; no LaTeX).
2. The problem was generated under these constraints:
%s
Does the problem comply with ALL of them? Answer YES or NO on line 2.
3. On line 3, list the features actually present in the problem, comma-separated, using only these names: %s.
Return exactly three lines and nothing else.`

// ErrEnvelopeMismatch marks a validator NO on the constraints line
// (the replacement for the old GRADE_MISMATCH).
var ErrEnvelopeMismatch = errors.New("ENVELOPE_MISMATCH")

// ValidateWordProblem checks a word problem's answer and envelope compliance
// in one LLM round-trip and extracts its observed features.
//
// featureNames is the closed list the validator may use on line 3 (the api
// side owns it). Returns the observed features on success.
func ValidateWordProblem(p *Problem, constraints string, featureNames []string) ([]string, error) {
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
	glog.Infof("OpenAI validation prompt = expected answer: %s = %s\n", prompt, p.Answer)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := chatCompletionWithRetry(
		context.Background(),
		client,
		openai.ChatCompletionRequest{
			Model: openai.GPT5,
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

	lines := strings.Split(strings.TrimSpace(resp.Choices[0].Message.Content), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("validator returned %d lines, want 3: %q",
			len(lines), resp.Choices[0].Message.Content)
	}

	answer := strings.TrimSpace(lines[0])
	if answer != p.Answer {
		msg := fmt.Sprintf("MISMATCH with validator: got %s, expected %s", answer, p.Answer)
		glog.Info(msg)
		return nil, errors.New(msg)
	}

	envelope := strings.TrimSpace(strings.ToUpper(lines[1]))
	if !strings.HasPrefix(envelope, "YES") {
		glog.Infof("%v: %s", ErrEnvelopeMismatch, p.Expression)
		return nil, ErrEnvelopeMismatch
	}

	var features []string
	if len(lines) >= 3 {
		for _, f := range strings.Split(lines[2], ",") {
			f = strings.ToLower(strings.TrimSpace(f))
			if f != "" {
				features = append(features, f)
			}
		}
	}
	return features, nil
}
