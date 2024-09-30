// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/common"
)

const PROMPT = `
Generate math questions in this format:
{
  "features": ["addition", "multiplication"]
  "expression": "3 + 2 * 3",
  "answer": "9",
  "explanation": "Following the order of operations (PEMDAS/BODMAS), you first perform the multiplication: 2×3=6 Then add 3+6=9",
  "difficulty": 8.3,
}
where "question" is the math question, "answer" is the correct answer, "explanation" is the explanation for the correct answer, "features" are the allowed features that were actually used in this problem, and "difficulty" is an age in years - this problem should be the appropriate difficulty for people of that age.
Return these problems as a valid JSON list with no additional text.
Do not wrap the JSON in markdown or any other JSON markers.
`
const OPENAI_URL = "https://api.openai.com/v1/completions"

func GenerateProblem(opts *Options) ([]Problem, error) {
	c, err := common.ReadConfig("conf.json")
	if err != nil {
		glog.Fatal(err)
	}

	sort.Strings(opts.Features)
	featuresJson, err := json.Marshal(opts.Features)
	if err != nil {
		glog.Fatal(err)
	}

	sort.Strings(opts.PreviousExpressions)
	previousExpressionsJson, err := json.Marshal(opts.PreviousExpressions)
	if err != nil {
		glog.Fatal(err)
	}

	prompt := PROMPT + fmt.Sprintf(
		"\nProduce %d problems in this format that may include these features %s and is the appropriate difficulty for a %g year old. ABSOLUTELY DO NOT produce any of the following expressions: %s",
		opts.NumProblems, featuresJson, opts.TargetDifficulty, previousExpressionsJson,
	)
	// log the prompt
	glog.Infof("OpenAI Prompt: %s\n", prompt)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			//Model: openai.GPT3Dot5TurboInstruct,
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)

	if err != nil {
		glog.Errorf("OpenAI error: %v\n", err)
		return []Problem{}, err
	}

	// log the response
	glog.Infof("OpenAI Response: %s\n", resp.Choices[0].Message.Content)

	var problems []Problem
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &problems)
	if err != nil {
		glog.Errorf("OpenAI content error: %v\n", err)
		return []Problem{}, err
	}

	return problems, nil
}
