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
	sql := fmt.Sprintf("problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0;",
		strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
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

// runHeuristicGenerator generates problems using the heuristic generator (addition/subtraction only).
// problemType must be a subset of ADDITION|SUBTRACTION. Returns the last new problem created,
// the count of new problems, and the set of unique IDs.
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
	if len(operations) == 0 {
		return nil, 0, uniqueIds
	}
	generatorOpts := &heuristic_generator.Options{
		Operations:       operations,
		Fractions:        false,
		Negatives:        false,
		TargetDifficulty: settings.TargetDifficulty,
	}
	for i := 0; i < numProblems; i++ {
		model := &Problem{}
		model.Generator = "heuristic_0.0"
		var err error
		model.Expression, model.Answer, model.Difficulty, err = heuristic_generator.GenerateProblem(generatorOpts)
		if err != nil {
			if _, ok := err.(*heuristic_generator.OptionsError); ok {
				glog.Errorf("%s Failed options validation: %v", logPrefix, err)
				return nil, newCount, uniqueIds
			}
			glog.Errorf("%s Couldn't generate problem: %v", logPrefix, err)
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(model.Expression))
		model.Id = h.Sum32()
		_, status, _, err := a.problemManager.Get(model.Id)
		if status != http.StatusNotFound {
			continue
		}
		uniqueIds[model.Id] = true
		model.ProblemTypeBitmap = 0
		if strings.Contains(model.Expression, "+") {
			model.ProblemTypeBitmap += uint64(ADDITION)
		}
		if strings.Contains(model.Expression, "-") {
			model.ProblemTypeBitmap += uint64(SUBTRACTION)
		}
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

func (a *Api) generateProblems(logPrefix string, c *gin.Context, settings *Settings, numProblems int) (*Problem, error) {
	var model *Problem
	var newProblem *Problem
	if settings.ProblemTypeBitmap == 0 {
		return nil, errors.New("settings.ProblemTypeBitmap is empty. Cannot generate problems.")
	}
	uniqueIds := map[uint32]bool{}
	newCount := 0
	inputProblemType := ProblemType(settings.ProblemTypeBitmap)
	// If difficulty is low and only using basic operations, use the heuristic generator.
	if settings.TargetDifficulty <= 5 && inputProblemType <= (ADDITION+SUBTRACTION) {
		newProblem, newCount, uniqueIds = a.runHeuristicGenerator(logPrefix, c, settings, numProblems, inputProblemType)
	} else {

		// Otherwise use the GPT generator.
		generatorOpts := &llm_generator.Options{
			Features:         ProblemTypeToFeatures(inputProblemType),
			TargetDifficulty: settings.TargetDifficulty,
			NumProblems:      numProblems, // we still return just one problem, but this lets us reduce the number of OpenAI calls we need to make
		}

		var err error
		var generatorProblems []llm_generator.Problem
		generatorProblems, err = llm_generator.GenerateProblem(generatorOpts)
		if err != nil {
			// Fall back to heuristic when OpenAI fails, as long as addition or subtraction is an option.
			heuristicType := inputProblemType & (ADDITION + SUBTRACTION)
			if heuristicType != 0 {
				glog.Infof("%s OpenAI failed (%v), falling back to heuristic generator (addition/subtraction)", logPrefix, err)
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
				model.Difficulty = p.Difficulty

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

				// Validate problem
				err = llm_generator.ValidateProblem(&p)
				if err != nil {
					//glog.Infof("%s problem validation failed: (%d: %s)", logPrefix, err)
					model = nil
					continue
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
