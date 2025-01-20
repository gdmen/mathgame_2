// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/common"
)

const (
	PROMPT_VALIDATION = "Return only the answer and no other text: %s"
)

func ValidateProblem(p *Problem) error {
	c, err := common.ReadConfig("conf.json")
	if err != nil {
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
		glog.Infof("%s", msg)
		return errors.New(msg)
	}
	return nil
}
