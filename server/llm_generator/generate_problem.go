// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here (bits, formula, pipeline, masks) REQUIRE updating that
// doc in the same PR. Formula changes also require a DifficultyVersion bump.
// Package llm_generator contains a math problem llm_generator
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"

	"garydmenezes.com/mathgame/server/common"
)

const (
	VERSION = "llm_0.6"
	// PROMPT_NARRATE turns scored symbolic skeletons into prose. The heuristic
	// owns the math and the difficulty; the model only dresses each skeleton in
	// a story that poses that exact computation - it invents no numbers and
	// changes no operation, so the stored difficulty (skeleton x word concept)
	// holds by construction. The skeleton list is appended as JSON.
	PROMPT_NARRATE = `You are given math computations, each with its numeric answer. For EACH one, write a short, self-contained word problem (a real-world story) whose solution requires EXACTLY that computation - the same numbers and the same operation(s) - and whose answer is exactly the given answer.
Rules:
- Do NOT introduce new numbers, and do NOT change, add, or drop operations. Use precisely the numbers and operations shown.
- Write the ENTIRE problem as prose inside \text{...}. Do NOT append the arithmetic the student must perform or its result: a problem posing "3 * 6" must NOT contain "3 * 6" or "= 18"; the student derives that.
- Use symbolic math outside \text{} ONLY when the computation itself is an equation to manipulate, e.g. \text{Solve for }x: 3x + 7 = 22.
- Division is the obelus (e.g. "60 ÷ 2"); fractions look like 3/8 or \frac{3}{8}. Return answers exactly as given.
Example input: [{"symbolic_expression": "60 ÷ 2", "answer": "30"}]
Example output: [{"symbolic_expression": "60 ÷ 2", "expression": "\\text{A 60-centimeter ribbon is cut into 2 equal pieces. How long is each piece?}", "explanation": "\\text{Divide the total length by the number of pieces: }60 \\div 2 = 30\\text{ centimeters.}"}]
Return a JSON list with one object per input, each {"symbolic_expression": ..., "expression": ..., "explanation": ...}. Echo "symbolic_expression" back EXACTLY as given (verbatim) so each narration can be matched to its computation. The "explanation" explains the solution and MAY show the arithmetic and result. Return ONLY the JSON list, no markdown.
Problems to narrate:
`
	// MAX_QUANTITY caps skeletons narrated per OpenAI call.
	MAX_QUANTITY = 20
)

// Skeleton is a scored symbolic problem for the narrator to dress in prose.
// SymbolicExpression is the canonical grammar form (division as ÷); Answer is
// its exact answer, both authoritative and copied verbatim onto the result.
// Features only seeds a topic-variety hint for the prompt; it is not stored.
type Skeleton struct {
	SymbolicExpression string
	Answer             string
	Features           []string
}

// NarrateProblems asks the model to write a word problem for each skeleton and
// returns one Problem per successfully narrated skeleton. Narrations are matched
// back to skeletons by the echoed symbolic_expression (see pairNarrations), not
// by position, so a dropped or reordered narration costs only its own item. The
// math is the skeleton's (SymbolicExpression + Answer copied verbatim); only the
// prose Expression and Explanation come from the model. The api side answer- and
// form-validates each narration against its skeleton before storing.
func NarrateProblems(skeletons []Skeleton) ([]Problem, error) {
	if len(skeletons) == 0 {
		return nil, nil
	}
	if len(skeletons) > MAX_QUANTITY {
		skeletons = skeletons[:MAX_QUANTITY]
	}

	c, err := common.ReadConfig("conf.json")
	if err != nil {
		// Return an error rather than fataling - the caller (generate_problems.go)
		// falls back to non-word generation when narration is unavailable.
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	type narrateInput struct {
		SymbolicExpression string `json:"symbolic_expression"`
		Answer             string `json:"answer"`
	}
	inputs := make([]narrateInput, len(skeletons))
	for i, s := range skeletons {
		inputs[i] = narrateInput{s.SymbolicExpression, s.Answer}
	}
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshal skeletons: %w", err)
	}
	prompt := PROMPT_NARRATE + string(inputJSON)
	// One topic-variety hint per batch, from the first skeleton that has one.
	for _, s := range skeletons {
		if h := firstTopicHint(s.Features); h != "" {
			prompt += h
			break
		}
	}
	glog.Infof("OpenAI narrate prompt: %s\n", prompt)

	client := openai.NewClient(c.OpenAiApiKey)
	resp, err := chatCompletionWithRetry(
		context.Background(),
		client,
		openai.ChatCompletionRequest{
			Model: openai.GPT5Nano,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
		},
	)
	if err != nil {
		glog.Errorf("OpenAI narrate error after retries: %v\n", err)
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI returned no choices")
	}

	var narrated []narratedItem
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &narrated); err != nil {
		glog.Errorf("OpenAI narrate content error: %v | %s\n", err, resp.Choices[0].Message.Content)
		return nil, err
	}
	return pairNarrations(skeletons, narrated), nil
}

// narratedItem is one object from the narrator's JSON list: the echoed
// symbolic_expression (used only to match the narration back to its skeleton)
// plus the prose the model wrote.
type narratedItem struct {
	SymbolicExpression string `json:"symbolic_expression"`
	Expression         string `json:"expression"`
	Explanation        string `json:"explanation"`
}

// pairNarrations matches each narration to its skeleton by the
// symbolic_expression the model echoes back, NOT by list position. The model
// occasionally drops or reorders an item; with positional zipping a single drop
// shifts every skeleton after it out of alignment, so the validator rejects the
// whole tail and yield collapses. Matching by echoed symbolic costs only the
// dropped item. First unused skeleton wins when a symbolic repeats in the batch;
// a narration whose echoed symbolic matches no remaining skeleton is dropped
// (the model altered the computation). The prose's math and bits still come from
// the matched skeleton, and the api-side validator remains the fidelity backstop.
func pairNarrations(skeletons []Skeleton, narrated []narratedItem) []Problem {
	bySymbolic := make(map[string][]int, len(skeletons))
	for i, s := range skeletons {
		bySymbolic[s.SymbolicExpression] = append(bySymbolic[s.SymbolicExpression], i)
	}
	used := make([]bool, len(skeletons))

	var out []Problem
	for _, n := range narrated {
		if strings.TrimSpace(n.Expression) == "" {
			continue
		}
		picked := -1
		for _, i := range bySymbolic[strings.TrimSpace(n.SymbolicExpression)] {
			if !used[i] {
				picked = i
				break
			}
		}
		if picked < 0 {
			glog.Infof("narrate: no skeleton for echoed symbolic %q; dropping", n.SymbolicExpression)
			continue
		}
		used[picked] = true
		s := skeletons[picked]
		out = append(out, Problem{
			Expression:         n.Expression,
			SymbolicExpression: s.SymbolicExpression,
			Answer:             s.Answer,
			Explanation:        n.Explanation,
		})
	}
	return out
}

// firstTopicHint returns a variety hint for the first feature that has one.
func firstTopicHint(features []string) string {
	for _, f := range features {
		if h := TopicPromptHint(f, rand.Intn); h != "" {
			return h
		}
	}
	return ""
}
