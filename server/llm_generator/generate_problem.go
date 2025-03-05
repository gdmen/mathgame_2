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
	OPENAI_URL      = "https://api.openai.com/v1/completions"
	PROMPT_QUESTION = `
Generate math questions in the format of this example:
{
  "features": ["addition", "multiplication"]
  "expression": "3 + 2 * 3",
  "answer": "9",
  "explanation": "\\text{Following the order of operations (PEMDAS/BODMAS), you first perform the multiplication: }2×3=6\\text{ Then add }3+6=9",
  "difficulty": 8.3,
}
or this example:
{
  "features": ["addition", "multiplication", "word"]
  "expression": "\\text{If a car travels at a speed of }60\\text{ miles per hour for }2\\text{ hours, how far does it travel?}"
  "answer": "120",
  "explanation": "\\text{The distance traveled is calculated by multiplying the speed by the time: }60\\text{ miles/hour }* 2\\text{ hours }= 120\\text{ miles.}",
  "difficulty": 15
}
where "question" is the math question in LaTeX math mode e.g. it might use \\text{} tags as shown, "answer" is the correct answer with no other text, "explanation" is the explanation for the correct answer in LaTeX math mode e.g. it might use \\text{} tags as shown, "features" are the allowed features that were actually used in this problem, and "difficulty" is an age in years - this problem should be the appropriate difficulty for people of that age.
Return these problems as a valid JSON list with no additional text.
Do not wrap the JSON in markdown or any other JSON markers.
`
	PROMPT_QUANTITY = "Produce %d unique %sproblems in this format that may include these features %s and is the appropriate difficulty for a %g year old."
	MAX_QUANTITY    = 20
)

func GenerateProblem(opts *Options) ([]Problem, error) {
	opts.NumProblems = common.Min(opts.NumProblems, MAX_QUANTITY)

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
	var ptype string
	for _, x := range opts.Features {
		if x == "word" {
			ptype = "word-"
			break
		}
	}
	prompt := PROMPT_QUESTION + "\n" + fmt.Sprintf(PROMPT_QUANTITY,
		opts.NumProblems, ptype, featuresJson, opts.TargetDifficulty,
	)
	glog.Infof("OpenAI GPT4oMini question prompt: %s\n", prompt)

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
	return problems, nil
}
