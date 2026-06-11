// Package llm_generator contains a math problem llm_generator
//
// Part of the problem-generation system - documented in docs/problem-generation.md.
// Behavior changes here REQUIRE updating that doc in the same PR.
package llm_generator // import "garydmenezes.com/mathgame/server/llm_generator"

// TopicPromptHint returns a short prompt hint for a specific topic to encourage
// variety in problem structure. Returns empty string if topic is not recognized.
var topicPromptHints = map[string][]string{
	"addition": {
		"Include a mix of: basic addition, missing addend problems (? + b = c), and addition with regrouping/carrying.",
		"Vary the format: some pure arithmetic, some with three or more addends, some comparing sums.",
		"Mix horizontal format (a + b) with word problems about combining groups.",
	},
	"subtraction": {
		"Include a mix of: basic subtraction, missing subtrahend (a - ? = c), and subtraction with borrowing.",
		"Vary between take-away problems, comparison problems (how many more), and missing number problems.",
		"Mix pure arithmetic with word problems about removing items or finding differences.",
	},
	"multiplication": {
		"Include a mix of: basic multiplication facts, multi-digit multiplication, and area/array problems.",
		"Vary between repeated addition framing, array/grid problems, and scaling problems.",
		"Mix times-table practice with word problems about equal groups and rates.",
	},
	"division": {
		"Include a mix of: basic division facts, long division, and division with remainders.",
		"Vary between sharing equally problems, grouping problems, and how-many-groups problems.",
		"Mix pure division with word problems about distributing items fairly.",
	},
	"fractions": {
		"Include a mix of: identifying fractions, comparing fractions, and fraction arithmetic.",
		"Vary between visual fraction problems, equivalent fraction problems, and mixed number problems.",
		"Mix fraction computation with word problems about parts of a whole or parts of a set.",
	},
	"word": {
		"Create multi-step word problems that require reading comprehension and mathematical reasoning.",
		"Use real-world contexts: shopping, cooking, sports, travel, building things.",
		"Vary the operations needed: some single-operation, some requiring two different operations.",
	},
}

// TopicPromptHint returns a random variety hint for the given topic feature name.
func TopicPromptHint(feature string, rng func(int) int) string {
	hints, ok := topicPromptHints[feature]
	if !ok || len(hints) == 0 {
		return ""
	}
	return "\n" + hints[rng(len(hints))] + "\n"
}
