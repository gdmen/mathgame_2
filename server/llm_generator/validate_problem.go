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
	PROMPT_VALIDATION            = "Return the answers to fractional expressions as fractions, not decimals. Return only the numeric answer and no other text: %s"
	PROMPT_VALIDATION_WITH_GRADE = "Given this math problem: %s\n1. Solve it. Return ONLY the numeric answer on line 1 (fractions as fractions, not decimals).\n2. Is this problem appropriate for %s (%s)? Answer YES or NO on line 2."
)

func ValidateProblem(p *Problem) error {
	return ValidateProblemWithGrade(p, 0)
}

func ValidateProblemWithGrade(p *Problem, gradeLevel int) error {
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

	var prompt string
	useGradeValidation := false
	if gradeLevel > 0 {
		cur, loadErr := loadCurriculum()
		if loadErr == nil {
			key := fmt.Sprintf("%d", gradeLevel)
			if grade, ok := cur.Grades[key]; ok {
				prompt = fmt.Sprintf(PROMPT_VALIDATION_WITH_GRADE, p.Expression, grade.Label, grade.Description)
				useGradeValidation = true
			}
		}
	}
	if !useGradeValidation {
		prompt = fmt.Sprintf(PROMPT_VALIDATION, p.Expression)
	}
	glog.Infof("OpenAI validation prompt = expected answer: %s = %s\n", prompt, p.Answer)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := client.CreateChatCompletion(
		context.Background(),
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
		glog.Infof("OpenAI error when validating: %v\n", err)
		return err
	}
	content := resp.Choices[0].Message.Content

	if useGradeValidation {
		// Parse two-line response: line 1 = answer, line 2 = YES/NO grade check
		lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
		answer := strings.TrimSpace(lines[0])
		if answer != p.Answer {
			msg := fmt.Sprintf("MISMATCH with OpenAI GPT4o validation: got %s, expected %s", answer, p.Answer)
			glog.Info(msg)
			return errors.New(msg)
		}
		if len(lines) > 1 {
			gradeCheck := strings.TrimSpace(strings.ToUpper(lines[1]))
			if strings.HasPrefix(gradeCheck, "NO") {
				msg := fmt.Sprintf("GRADE_MISMATCH: problem not appropriate for grade %d: %s", gradeLevel, p.Expression)
				glog.Info(msg)
				return errors.New(msg)
			}
		}
	} else {
		if content != p.Answer {
			msg := fmt.Sprintf("MISMATCH with OpenAI GPT4o validation: %s", content)
			glog.Info(msg)
			return errors.New(msg)
		}
	}
	return nil
}
