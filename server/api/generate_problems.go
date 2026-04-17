// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	heuristic_generator "garydmenezes.com/mathgame/server/generator"
	llm_generator "garydmenezes.com/mathgame/server/llm_generator"
)

const (
	// difficulty comparison epsilon as a multiple
	problemSelectionEpsilon = 0.3
	// minimum number of problems we want to select from
	minSelectionPool = 100
)

// Get all problem ids that satisfy this ProblemTypeBitmap and have similar Difficulty
func (a *Api) getSatisfyingProblemIds(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	// Special case to guarantee word problems if they're turned on
	if (ProblemType(settings.ProblemTypeBitmap) & WORD) != 0 {
		res := []ProblemType{}
		for _, pt := range permutations {
			if (pt & WORD) != 0 {
				res = append(res, pt)
			}
		}
		permutations = res
	}
	if len(permutations) == 0 {
		return &([]uint32{}), nil
	}

	diffLowerBound := settings.TargetDifficulty * (1 - problemSelectionEpsilon)
	diffUpperBound := settings.TargetDifficulty * (1 + problemSelectionEpsilon)
	// Grade filter: grade_level > 0 always-excludes backfilled/ungraded problems (grade=0).
	// grade_level = settings.GradeLevel ensures cross-grade pool isolation.
	sql := fmt.Sprintf("problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0 AND grade_level > 0 AND grade_level = %d;",
		strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
		settings.GradeLevel,
	)
	if len(*prevIds) > 0 {
		idFilter := fmt.Sprintf("id NOT IN (%s) AND ", strings.Replace(strings.Trim(fmt.Sprint(*prevIds), "[]"), " ", ",", -1))
		sql = idFilter + sql
	}
	glog.Infof("getSatisfyingProblemIds sql: select * from problems where %s\n", sql)
	problemIds, status, msg, err := a.problemManager.CustomIdList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problemIds) != nil {
		return nil, err
	}
	return problemIds, nil
}

func (a *Api) selectProblem(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32) (*Problem, error) {
	// Check spaced repetition review queue first
	dueReviewID := a.getDueReviewProblem(logPrefix, settings.UserId)
	if dueReviewID != 0 {
		p, status, msg, err := a.problemManager.Get(dueReviewID)
		if err == nil && status == http.StatusOK && !p.Disabled {
			glog.Infof("%s serving spaced rep review problem=%d", logPrefix, dueReviewID)
			return p, nil
		}
		glog.Infof("%s spaced rep problem=%d unavailable: %s %v", logPrefix, dueReviewID, msg, err)
	}

	// Try topic-weighted selection first
	topicStats, tsErr := a.getTopicStats(settings.UserId)
	if tsErr != nil {
		glog.Errorf("%s getTopicStats: %v (falling back to default selection)", logPrefix, tsErr)
		topicStats = nil
	}

	if topicStats != nil && len(topicStats) > 0 {
		targetTopic, topicDiff := chooseWeightedTopic(topicStats, settings.ProblemTypeBitmap, settings.TargetDifficulty, rand.Intn)
		if targetTopic != 0 {
			glog.Infof("%s topic-weighted selection: topic=%d difficulty=%.2f", logPrefix, targetTopic, topicDiff)
			// Build a topic-specific settings to query with the topic's difficulty
			topicSettings := *settings
			topicSettings.TargetDifficulty = topicDiff
			// Only include permutations that contain the target topic
			topicSettings.ProblemTypeBitmap = settings.ProblemTypeBitmap // keep all enabled, filter below
			pids, err := a.getSatisfyingProblemIdsForTopic(logPrefix, c, &topicSettings, prevIds, targetTopic)
			if err == nil && len(*pids) > 0 {
				pid := (*pids)[rand.Intn(len(*pids))]
				p, status, msg, err := a.problemManager.Get(pid)
				if err == nil && status == http.StatusOK {
					return p, nil
				}
				glog.Infof("%s topic-weighted fetch failed (id=%d): %s : %v", logPrefix, pid, msg, err)
			}
			// Topic-specific pool too small, trigger background generation
			if err == nil && len(*pids) < minSelectionPool {
				glog.Infof("%s topic pool small (%d), generating more", logPrefix, len(*pids))
				a.generateProblemsBackground(logPrefix, c, &topicSettings)
			}
		}
	}

	// Fall back to default (non-topic-weighted) selection
	pids, err := a.getSatisfyingProblemIds(logPrefix, c, settings, prevIds)
	if err != nil {
		return nil, err
	}

	if len(*pids) < minSelectionPool {
		glog.Infof("%s generating new problems because there are only %d problems", logPrefix, len(*pids))
		a.generateProblemsBackground(logPrefix, c, settings)
	}

	if len(*pids) > 0 {
		pid := (*pids)[rand.Intn(len(*pids))]
		p, status, msg, err := a.problemManager.Get(pid)
		if HandleMngrResp(logPrefix, c, status, msg, err, p) != nil {
			glog.Infof("%s unexpected (recoverable) error fetching problem (id=%d): %s : %s", logPrefix, pid, msg, err)
		} else {
			return p, nil
		}
	}
	return a.generateProblem(logPrefix, c, settings)
}

// getSatisfyingProblemIdsForTopic is like getSatisfyingProblemIds but only returns
// problems whose bitmap contains the target topic.
func (a *Api) getSatisfyingProblemIdsForTopic(logPrefix string, c *gin.Context, settings *Settings, prevIds *[]uint32, targetTopic uint64) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	// Filter to permutations containing the target topic
	filtered := []ProblemType{}
	for _, pt := range permutations {
		if (uint64(pt) & targetTopic) != 0 {
			filtered = append(filtered, pt)
		}
	}
	if len(filtered) == 0 {
		empty := []uint32{}
		return &empty, nil
	}

	diffLowerBound := settings.TargetDifficulty * (1 - problemSelectionEpsilon)
	diffUpperBound := settings.TargetDifficulty * (1 + problemSelectionEpsilon)
	// Grade filter: always-exclude backfilled/ungraded (grade=0); match settings.GradeLevel.
	sql := fmt.Sprintf("problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0 AND grade_level > 0 AND grade_level = %d;",
		strings.Replace(strings.Trim(fmt.Sprint(filtered), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
		settings.GradeLevel,
	)
	if len(*prevIds) > 0 {
		idFilter := fmt.Sprintf("id NOT IN (%s) AND ", strings.Replace(strings.Trim(fmt.Sprint(*prevIds), "[]"), " ", ",", -1))
		sql = idFilter + sql
	}
	glog.Infof("getSatisfyingProblemIdsForTopic sql: select id from problems where %s\n", sql)
	problemIds, status, msg, err := a.problemManager.CustomIdList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problemIds) != nil {
		return nil, err
	}
	return problemIds, nil
}

func (a *Api) generateProblem(logPrefix string, c *gin.Context, settings *Settings) (*Problem, error) {
	retries := 5
	var err error
	var p *Problem
	for i := 0; i < retries; i++ {
		p, err = a.generateProblems(logPrefix, c, settings, 5)
		if p != nil {
			return p, nil
		}
	}
	return nil, err
}

func (a *Api) generateProblemsBackground(logPrefix string, c *gin.Context, settings *Settings) error {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				glog.Infof("%s a.generateProblems failed: %s", logPrefix, r)
			}
		}()

		a.generateProblems(logPrefix, c, settings, 20)
	}()

	// job successfully 'enqueued'
	return nil

}

// runHeuristicGenerator generates problems using the heuristic generator.
// Supports ADDITION, SUBTRACTION, MULTIPLICATION, DIVISION, FRACTIONS, NEGATIVES.
// WORD problems should be generated via the LLM generator instead.
// Returns the last new problem created, the count of new problems, and the set
// of unique IDs.
func (a *Api) runHeuristicGenerator(logPrefix string, c *gin.Context, settings *Settings, numProblems int, problemType ProblemType) (*Problem, int, map[uint32]bool) {
	uniqueIds := map[uint32]bool{}
	newCount := 0
	var newProblem *Problem
	operations := []string{}
	if (ADDITION & problemType) > 0 {
		operations = append(operations, "+")
	}
	if (SUBTRACTION & problemType) > 0 {
		operations = append(operations, "-")
	}
	if (MULTIPLICATION & problemType) > 0 {
		operations = append(operations, "*")
	}
	if (DIVISION & problemType) > 0 {
		operations = append(operations, "/")
	}
	if len(operations) == 0 {
		return nil, 0, uniqueIds
	}
	generatorOpts := &heuristic_generator.Options{
		Operations:       operations,
		Fractions:        (FRACTIONS & problemType) > 0,
		Negatives:        (NEGATIVES & problemType) > 0,
		TargetDifficulty: settings.TargetDifficulty,
		GradeLevel:       settings.GradeLevel,
	}
	for i := 0; i < numProblems; i++ {
		model := &Problem{}
		model.Generator = heuristic_generator.VERSION
		var err error
		var heuristicDiff float64
		model.Expression, model.Answer, heuristicDiff, err = heuristic_generator.GenerateProblem(generatorOpts)
		if err != nil {
			if _, ok := err.(*heuristic_generator.OptionsError); ok {
				glog.Errorf("%s Failed options validation: %v", logPrefix, err)
				return nil, newCount, uniqueIds
			}
			glog.Errorf("%s Couldn't generate problem: %v", logPrefix, err)
			continue
		}
		// Pin difficulty to requested target (heuristic scale differs from LLM scale)
		model.Difficulty = settings.TargetDifficulty
		model.GradeLevel = settings.GradeLevel
		glog.Infof("%s heuristic problem: %s = %s (grade=%d pinned_diff=%g raw=%g)", logPrefix, model.Expression, model.Answer, model.GradeLevel, model.Difficulty, heuristicDiff)
		h := fnv.New32a()
		h.Write([]byte(model.Expression))
		model.Id = h.Sum32()
		_, status, _, err := a.problemManager.Get(model.Id)
		if status != http.StatusNotFound {
			continue
		}
		uniqueIds[model.Id] = true
		model.ProblemTypeBitmap = detectProblemTypeBitmap(model.Expression)
		status, msg, err := a.problemManager.Create(model)
		if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
			glog.Infof("%s could not create problem: (%d: %s)", logPrefix, status, err)
			continue
		}
		newCount++
		newProblem = model
	}
	return newProblem, newCount, uniqueIds
}

// detectProblemTypeBitmap inspects a generated expression and returns the
// bitmap of problem types it contains. Used to tag heuristic-generated
// problems so they're selected correctly by type filter.
//
// Heuristic generator formatting conventions:
//   - Binary operators are surrounded by spaces: "42 / 6", "3 + 5"
//   - Fractions have no surrounding spaces on their slash: "1/2 + 3/4"
//   - Negative numbers appear as "-N" or with a leading minus in expressions.
func detectProblemTypeBitmap(expr string) uint64 {
	var bitmap uint64 = 0
	// Subtraction has a space-dash-space pattern; a bare leading "-" is a
	// negative number, not a subtraction operator.
	if strings.Contains(expr, " - ") {
		bitmap |= uint64(SUBTRACTION)
	}
	if strings.Contains(expr, " + ") {
		bitmap |= uint64(ADDITION)
	}
	if strings.Contains(expr, " * ") {
		bitmap |= uint64(MULTIPLICATION)
	}
	if strings.Contains(expr, " / ") {
		bitmap |= uint64(DIVISION)
	}
	// Fractions: a "/" without surrounding spaces (e.g., "1/2").
	// Easy test: any "/" that isn't part of " / ".
	if strings.Count(expr, "/") > strings.Count(expr, " / ") {
		bitmap |= uint64(FRACTIONS)
	}
	// Negatives: presence of a unary minus (leading "-" or "-" after operator/space).
	// Cheap heuristic: "-" at start, or " -" appearing where no subtraction operator follows.
	if strings.HasPrefix(expr, "-") {
		bitmap |= uint64(NEGATIVES)
	}
	return bitmap
}

func (a *Api) generateProblems(logPrefix string, c *gin.Context, settings *Settings, numProblems int) (*Problem, error) {
	var model *Problem
	var newProblem *Problem
	if settings.ProblemTypeBitmap == 0 {
		return nil, errors.New("settings.ProblemTypeBitmap is empty. Cannot generate problems.")
	}
	uniqueIds := map[uint32]bool{}
	newCount := 0
	inputProblemType := ProblemType(settings.ProblemTypeBitmap)
	// Use the heuristic generator unless the user has enabled WORD problems.
	// Heuristic_1.0 supports all 4 basic operations, fractions, and negatives
	// and is grade-aware via settings.GradeLevel. Word problems still go to
	// the LLM because they need natural language and contextual variety.
	if (inputProblemType & WORD) == 0 {
		newProblem, newCount, uniqueIds = a.runHeuristicGenerator(logPrefix, c, settings, numProblems, inputProblemType)
	} else {

		// Otherwise use the GPT generator.
		generatorOpts := &llm_generator.Options{
			Features:         ProblemTypeToFeatures(inputProblemType),
			TargetDifficulty: settings.TargetDifficulty,
			NumProblems:      numProblems, // we still return just one problem, but this lets us reduce the number of OpenAI calls we need to make
			GradeLevel:       settings.GradeLevel,
		}

		var err error
		var generatorProblems []llm_generator.Problem
		generatorProblems, err = llm_generator.GenerateProblem(generatorOpts)
		if err != nil {
			// Fall back to heuristic when OpenAI fails. Strip WORD since the
			// heuristic doesn't produce word problems, and fall back on the
			// remaining arithmetic types.
			heuristicType := inputProblemType &^ WORD
			if heuristicType != 0 {
				glog.Infof("%s OpenAI failed (%v), falling back to heuristic generator", logPrefix, err)
				newProblem, newCount, uniqueIds = a.runHeuristicGenerator(logPrefix, c, settings, numProblems, heuristicType)
			} else {
				msg := "Couldn't generate problems"
				glog.Errorf("%s %s: %v", logPrefix, msg, err)
				return nil, err
			}
		} else {
			for _, p := range generatorProblems {
				glog.Infof("%s generated problem: %v", logPrefix, p)

				// Convert to an api.Problem
				model = &Problem{}
				model.Generator = llm_generator.VERSION
				model.ProblemTypeBitmap = uint64(FeaturesToProblemType(p.Features))
				model.Expression = strings.TrimSpace(p.Expression)
				model.Answer = p.Answer
				model.Explanation = p.Explanation
				// Pin difficulty to requested target (LLM self-reported difficulty varies)
				model.Difficulty = settings.TargetDifficulty
				model.GradeLevel = settings.GradeLevel
				glog.Infof("%s LLM problem: pinned difficulty=%g grade=%d (LLM raw=%g)", logPrefix, model.Difficulty, model.GradeLevel, p.Difficulty)

				// Use expression hash as model.Id
				h := fnv.New32a()
				h.Write([]byte(model.Expression))
				model.Id = h.Sum32()

				// Check for collisions
				_, status, _, err := a.problemManager.Get(model.Id)
				// There is certainly no collision iff we receive a 404
				if status != http.StatusNotFound {
					//glog.Infof("%s could not verify uniqueness of problem: (%d: %v)", logPrefix, status, err)
					model = nil
					continue
				}
				uniqueIds[model.Id] = true

				// Validate problem (includes grade alignment check if grade is set)
				err = llm_generator.ValidateProblemWithGrade(&p, settings.GradeLevel)
				if err != nil {
					glog.Infof("%s problem validation failed: %v", logPrefix, err)
					model = nil
					continue
				}

				// Difficulty calibration: reject if LLM's self-reported difficulty
				// diverges too far from what we requested (> 100% off)
				if settings.TargetDifficulty > 0 && p.Difficulty > 0 {
					ratio := p.Difficulty / settings.TargetDifficulty
					if ratio < 0.5 || ratio > 2.0 {
						glog.Infof("%s difficulty calibration reject: requested=%.1f, LLM reported=%.1f (ratio=%.2f)",
							logPrefix, settings.TargetDifficulty, p.Difficulty, ratio)
						model = nil
						continue
					}
				}

				// Write to database
				status, msg, err := a.problemManager.Create(model)
				if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
					glog.Infof("%s could not create problem: (%d: %s)", logPrefix, status, err)
					model = nil
					continue
				}
				newCount += 1
				newProblem = model
			}
		}
	}

	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d and new problems generated: %d", logPrefix, numProblems, len(uniqueIds), newCount)

	// Just return the last problem added
	if newProblem == nil {
		return nil, errors.New("Failed to produce any valid new problem.")
	}
	return newProblem, nil
}
