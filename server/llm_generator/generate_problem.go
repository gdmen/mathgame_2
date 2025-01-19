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

const (
	PROMPT_QUESTION = `
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
	PROMPT_QUANTITY   = "Produce %d unique problems in this format that may include these features %s and is the appropriate difficulty for a %g year old."
	PROMPT_VALIDATION = "return only the answer to: %s"
	OPENAI_URL        = "https://api.openai.com/v1/completions"
)

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

	// Request question(s)
	prompt := PROMPT_QUESTION + "\n" + fmt.Sprintf(PROMPT_QUANTITY,
		opts.NumProblems, featuresJson, opts.TargetDifficulty,
	)
	glog.Infof("OpenAI question prompt: %s\n", prompt)

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

	var problems []Problem
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &problems)
	if err != nil {
		glog.Errorf("OpenAI content error: %v | %s\n", err, resp.Choices[0].Message.Content)
		return []Problem{}, err
	}

	// Validate question(s)
	validated := []Problem{}
	for _, p := range problems {
		prompt = fmt.Sprintf(PROMPT_VALIDATION, p.Expression)
		glog.Infof("OpenAI validation prompt: %s\n", prompt)

		resp, err := client.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
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
			glog.Infof("OpenAI error when validating: %v\n", err)
			continue
		}
		glog.Infof("OpenAI validation resp: %s", resp.Choices[0].Message.Content)
		validated = append(validated, p)
	}

	return problems, nil
}
