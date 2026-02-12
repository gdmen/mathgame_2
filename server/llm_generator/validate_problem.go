// Package llm_generator contains a math problem llm_generator
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

const (
	PROMPT_VALIDATION = "Return the answers to fractional expressions as fractions, not decimals. Return only the numeric answer and no other text: %s"
)

func ValidateProblem(p *Problem) error {
	if strings.ContainsAny(p.Answer, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		msg := fmt.Sprintf("Answer contained text: %v\n", p)
		glog.Info(msg)
		return errors.New(msg)
	}

	c, err := common.ReadConfig("conf.json")
	if err != nil {
		glog.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		glog.Fatal(err)
	}

	prompt := fmt.Sprintf(PROMPT_VALIDATION, p.Expression)
	glog.Infof("OpenAI validation prompt = expected answer: %s = %s\n", prompt, p.Answer)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4o,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)

	if err != nil {
		glog.Infof("OpenAI error when validating: %v\n", err)
		return err
	}
	content := resp.Choices[0].Message.Content
	if content != p.Answer {
		msg := fmt.Sprintf("MISMATCH with OpenAI GPT4o validation: %s", content)
		glog.Info(msg)
		return errors.New(msg)
	}
	return nil
}
