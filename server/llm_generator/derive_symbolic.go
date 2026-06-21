// Package llm_generator contains a math problem llm_generator
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here REQUIRE updating that doc in the same PR.
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/common"
)

// PROMPT_DERIVE_SYMBOLIC asks for the bare symbolic form a word problem
// expresses - the same operations and numbers, so its difficulty matches the
// problem. A find-the-unknown problem becomes a single-unknown equation, which
// the pipeline folds to a '?' (MISSING_NUMBER), so its inverse structure
// survives instead of collapsing to plain arithmetic. No answer is supplied, so
// the model transcribes rather than works backward; the caller verifies the
// result evaluates to the stored answer.
const PROMPT_DERIVE_SYMBOLIC = `Return ONLY the bare symbolic form of the computation the following word problem asks for: the same operations and numbers it describes, with no prose and no \text.
- A direct computation is just the expression, e.g. "60 * 2", "9999 / 3 / 3".
- A find-the-unknown problem (where the missing value is what's asked for) is an equation using a single unknown "x", e.g. "x - 5 = 10", "x / 4 = 3". Use at most one unknown.
Word problem: %s`

// DeriveSymbolicExpression derives a word problem's symbolic_expression from its
// prose, using GPT5 by default.
func DeriveSymbolicExpression(p *Problem) (string, error) {
	return DeriveSymbolicExpressionWithModel(p, openai.GPT5)
}

// DeriveSymbolicExpressionWithModel is DeriveSymbolicExpression with an explicit
// model, for bulk tools that trade per-call accuracy for cost.
func DeriveSymbolicExpressionWithModel(p *Problem, model string) (string, error) {
	c, err := common.ReadConfig("conf.json")
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return "", fmt.Errorf("validate config: %w", err)
	}

	prompt := fmt.Sprintf(PROMPT_DERIVE_SYMBOLIC, p.Expression)
	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := chatCompletionWithRetry(
		context.Background(),
		client,
		openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		},
	)
	if err != nil {
		return "", err
	}

	form := cleanDerivedForm(resp.Choices[0].Message.Content)
	if form == "" {
		return "", fmt.Errorf("empty symbolic_expression for %q", p.Expression)
	}
	glog.Infof("derived symbolic_expression %q for %q", form, p.Expression)
	return form, nil
}

// cleanDerivedForm pulls the bare expression out of the model's reply: the first
// line, stripped of surrounding whitespace and code-fence/quote wrappers.
// Anything still malformed is caught downstream by the lexer + answer check.
func cleanDerivedForm(content string) string {
	form := strings.TrimSpace(content)
	if i := strings.IndexByte(form, '\n'); i >= 0 {
		form = form[:i]
	}
	return strings.Trim(strings.TrimSpace(form), "`\"")
}
