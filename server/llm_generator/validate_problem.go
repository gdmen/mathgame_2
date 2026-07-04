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
// zero LLM calls. The skeleton owns the math and the bits, so the validator
// only confirms the two fidelities the skeleton cannot self-certify - that the
// prose still solves to the skeleton's answer and still poses the skeleton's
// computation:
//
//	line 1: the numeric answer (must equal the skeleton's)
//	line 2: YES/NO - does the prose use the skeleton's operations and numbers?
const PROMPT_VALIDATION_WORD = `Given this word problem: %s
1. Solve it. Return ONLY the numeric answer on line 1 (fractions as fractions, not decimals; no LaTeX).
2. The intended computation is %q. On line 2, answer YES only if the problem uses the operations and numbers this computation actually requires, NO otherwise.
Return exactly 2 lines and nothing else.`

// ErrFormMismatch marks a validator NO on the form line: the prose doesn't pose
// the skeleton's actual computation.
var ErrFormMismatch = errors.New("FORM_MISMATCH")

// ValidateWordProblem confirms a narrated word problem against its skeleton in
// one LLM round-trip: the prose must solve to the skeleton's answer and pose
// the skeleton's computation. Returns nil on success; any mismatch is an error
// (fail closed). Uses GPT5 - the stronger model - as the independent check on
// the cheaper narrator.
func ValidateWordProblem(p *Problem) error {
	if strings.ContainsAny(p.Answer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		msg := fmt.Sprintf("Answer contained text: %v\n", p)
		glog.Info(msg)
		return errors.New(msg)
	}

	c, err := common.ReadConfig("conf.json")
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	prompt := fmt.Sprintf(PROMPT_VALIDATION_WORD, p.Expression, p.SymbolicExpression)
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
		return err
	}

	if err := parseValidatorResponse(resp.Choices[0].Message.Content, p); err != nil {
		glog.Infof("validator reject: %v (%q)", err, p.Expression)
		return err
	}
	return nil
}

// parseValidatorResponse interprets the validator's two lines: the answer (1,
// must equal the skeleton's) and the form YES/NO (2, does the prose pose the
// skeleton's computation). Returns an error for an answer mismatch or a form NO.
func parseValidatorResponse(content string, p *Problem) error {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) < 2 {
		return fmt.Errorf("validator returned %d lines: %q", len(lines), content)
	}
	if answer := strings.TrimSpace(lines[0]); answer != p.Answer {
		return fmt.Errorf("validator answer %q, expected %q", answer, p.Answer)
	}
	if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(lines[1])), "YES") {
		return ErrFormMismatch
	}
	return nil
}
