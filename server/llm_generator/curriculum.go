package llm_generator

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

//go:embed curriculum.json
var curriculumFS embed.FS

type CurriculumExample struct {
	Expression string  `json:"expression"`
	Answer     string  `json:"answer"`
	Difficulty float64 `json:"difficulty"`
}

type GradeConfig struct {
	Label       string              `json:"label"`
	Strands     []string            `json:"strands"`
	Operations  []string            `json:"operations"`
	Description string              `json:"description"`
	Examples    []CurriculumExample `json:"examples"`
}

type CurriculumConfig struct {
	Grades map[string]GradeConfig `json:"grades"`
}

var curriculum *CurriculumConfig

func loadCurriculum() (*CurriculumConfig, error) {
	if curriculum != nil {
		return curriculum, nil
	}
	data, err := curriculumFS.ReadFile("curriculum.json")
	if err != nil {
		return nil, fmt.Errorf("reading curriculum.json: %w", err)
	}
	c := &CurriculumConfig{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parsing curriculum.json: %w", err)
	}
	curriculum = c
	return curriculum, nil
}

// GradeContext returns a prompt suffix with grade-level context and few-shot
// examples. Returns empty string if gradeLevel is 0 (not set) or not found.
func GradeContext(gradeLevel int) string {
	if gradeLevel == 0 {
		return ""
	}
	c, err := loadCurriculum()
	if err != nil {
		return ""
	}
	key := strconv.Itoa(gradeLevel)
	grade, ok := c.Grades[key]
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\nThis student is in %s (Common Core strands: %s). ",
		grade.Label, strings.Join(grade.Strands, ", ")))
	sb.WriteString(grade.Description)
	sb.WriteString("\nHere are example problems at this grade level:\n")
	for _, ex := range grade.Examples {
		sb.WriteString(fmt.Sprintf("  Expression: %s, Answer: %s\n", ex.Expression, ex.Answer))
	}
	sb.WriteString("Generate problems that match this grade level's curriculum.\n")
	return sb.String()
}

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
