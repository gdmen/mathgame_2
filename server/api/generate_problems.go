// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	heuristic_generator "garydmenezes.com/mathgame/server/generator"
	llm_generator "garydmenezes.com/mathgame/server/llm_generator"
)

const (
	// difficulty comparison epsilon as a multiple
	problemSelectionEpsilon = 0.1
	// minimum number of problems we want to select from
	minSelectionPool = 100
)

// Get all problem ids that satisfy this ProblemTypeBitmap and have similar Difficulty
func (a *Api) getSatisfyingProblemIds(logPrefix string, c *gin.Context, settings *Settings, prevId uint32) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	if len(permutations) == 0 {
		return &([]uint32{}), nil
	}
	diffLowerBound := settings.TargetDifficulty * (1 - problemSelectionEpsilon)
	diffUpperBound := settings.TargetDifficulty * (1 + problemSelectionEpsilon)
	sql := fmt.Sprintf(
		"id != %d AND problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0;",
		prevId,
		strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
	)
	glog.Infof("sql: %s\n", sql)
	problem_ids, status, msg, err := a.problemManager.CustomIdList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problem_ids) != nil {
		return nil, err
	}
	return problem_ids, nil
}

func (a *Api) selectProblem(logPrefix string, c *gin.Context, settings *Settings, prevId uint32) (*Problem, error) {
	pids, err := a.getSatisfyingProblemIds(logPrefix, c, settings, prevId)
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
		p, err = a.generateProblems(logPrefix, c, settings, 1)
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

func (a *Api) generateProblems(logPrefix string, c *gin.Context, settings *Settings, numProblems int) (*Problem, error) {
	var model *Problem
	if settings.ProblemTypeBitmap == 0 {
		return nil, errors.New("settings.ProblemTypeBitmap is empty. Cannot generate problems.")
	}
	uniqueIds := map[uint32]bool{}
	newCount := 0
	inputProblemType := ProblemType(settings.ProblemTypeBitmap)
	// If difficulty is low and only using basic operations, use the heuristic generator.
	if settings.TargetDifficulty <= 5 && inputProblemType <= (ADDITION+SUBTRACTION) {
		for i := 0; i < numProblems; i++ {
			model = &Problem{}
			model.Generator = "heuristic_0.0"
			// TODO: this is temporary logic to make the generator compatible with the new ProblemTypeBitmap
			fractions := false //(FRACTIONS & inputProblemType) > 0
			negatives := false //(NEGATIVES & inputProblemType) > 0
			operations := []string{}
			if (ADDITION & inputProblemType) > 0 {
				operations = append(operations, "+")
			}
			if (SUBTRACTION & inputProblemType) > 0 {
				operations = append(operations, "-")
			}
			generator_opts := &heuristic_generator.Options{
				Operations:       operations,
				Fractions:        fractions,
				Negatives:        negatives,
				TargetDifficulty: settings.TargetDifficulty,
			}
			var err error
			model.Expression, model.Answer, model.Difficulty, err = heuristic_generator.GenerateProblem(generator_opts)
			if err != nil {
				if err, ok := err.(*heuristic_generator.OptionsError); ok {
					msg := "Failed options validation"
					glog.Errorf("%s %s: %v", logPrefix, msg, err)
					return nil, err
				}
				msg := "Couldn't generate problem"
				glog.Errorf("%s %s: %v", logPrefix, msg, err)
				continue
			}
			// Use expression hash as model.Id
			h := fnv.New32a()
			h.Write([]byte(model.Expression))
			model.Id = h.Sum32()

			// Check for collisions
			_, status, _, err := a.problemManager.Get(model.Id)
			// There is no collision iff we receive a 404
			if status != http.StatusNotFound {
				//msg := fmt.Sprintf("Could not verify uniqueness of problem: (%d: %v)", status, err)
				//glog.Infof("%s %s", logPrefix, msg)
				model = nil
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
			// Write to database
			status, msg, err := a.problemManager.Create(model)
			if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
				glog.Infof("%s could not create problem: (%d: %s)", logPrefix, status, err)
				continue
			}
			newCount += 1
		}
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
			msg := "Couldn't generate problems"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			return nil, err
		}

		re_whitespace := regexp.MustCompile(`\s+`)
		for _, p := range generatorProblems {
			glog.Infof("%s generated problem: %v", logPrefix, p)

			// Convert to an api.Problem
			model = &Problem{}
			model.Generator = "llm_0.0"
			model.ProblemTypeBitmap = uint64(FeaturesToProblemType(p.Features))
			model.Expression = re_whitespace.ReplaceAllString(p.Expression, "")
			model.Answer = p.Answer
			model.Explanation = p.Explanation
			model.Difficulty = p.Difficulty

			// Use expression hash as model.Id
			h := fnv.New32a()
			h.Write([]byte(model.Expression))
			model.Id = h.Sum32()

			// Check for collisions
			_, status, _, err := a.problemManager.Get(model.Id)
			// There is no collision iff we receive a 404
			if status != http.StatusNotFound {
				//glog.Infof("%s could not verify uniqueness of problem: (%d: %v)", logPrefix, status, err)
				//model = nil
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
		}
	}

	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d", logPrefix, numProblems, len(uniqueIds))
	glog.Infof("%s generator numProblems requested: %d vs new problems generated: %d", logPrefix, numProblems, newCount)

	// Just return the last problem added
	if model == nil {
		return nil, errors.New("Failed to produce any valid new problem.")
	}
	return model, nil
}
