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

	"garydmenezes.com/mathgame/server/common"
	llm_generator "garydmenezes.com/mathgame/server/llm_generator"
)

const (
	// difficulty comparison epsilon as a multiple
	problemSelectionEpsilon = 0.1
	// minimum number of problems we want to select from
	minSelectionPool = 100
)

// Get all problem ids that satisfy this ProblemTypeBitmap and have similar Difficulty
func (a *Api) getSatisfyingProblemIds(logPrefix string, c *gin.Context, settings *Settings) (*[]uint32, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	if len(permutations) == 0 {
		return &([]uint32{}), nil
	}
	diffLowerBound := settings.TargetDifficulty * (1 - problemSelectionEpsilon)
	diffUpperBound := settings.TargetDifficulty * (1 + problemSelectionEpsilon)
	sql := fmt.Sprintf(
		"problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0;",
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

func (a *Api) selectProblem(logPrefix string, c *gin.Context, settings *Settings) (*Problem, error) {
	pids, err := a.getSatisfyingProblemIds(logPrefix, c, settings)
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
			glog.Infof("%s unexpected (recoverable) error fetching problem (id=%s): %s : %s", logPrefix, pid, msg, err)
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
	// TODO: If difficulty is less than 5 and only using basic operations, use the heuristic generator.

	// Otherwise use the GPT generator.
	generatorOpts := &llm_generator.Options{
		Features:         ProblemTypeToFeatures(ProblemType(settings.ProblemTypeBitmap)),
		TargetDifficulty: settings.TargetDifficulty,
		NumProblems:      numProblems, // we still return just one problem, but this lets us reduce the number of OpenAI calls we need to make
	}

	var err error
	var generatorProblems []llm_generator.Problem
	generatorProblems, err = llm_generator.GenerateProblem(generatorOpts)
	if err != nil {
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusInternalServerError, common.GetError(msg))
		return nil, err
	}

	re_whitespace := regexp.MustCompile(`\s+`)
	uniqueIds := map[uint32]bool{}
	newCount := 0
	for _, p := range generatorProblems {
		glog.Infof("%s generated problem: %v", logPrefix, p)

		// Convert to an api.Problem
		model = &Problem{}
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
			glog.Infof("%s could not verify uniqueness of problem: (%d: %v)", logPrefix, status, err)
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
	}
	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d", logPrefix, numProblems, len(uniqueIds))
	glog.Infof("%s generator numProblems requested: %d vs new problems generated: %d", logPrefix, numProblems, newCount)

	// Just return the last problem added
	if model == nil {
		return nil, errors.New("Failed to produce any valid new problem.")
	}
	return model, nil
}
