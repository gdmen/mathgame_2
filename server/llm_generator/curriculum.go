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
