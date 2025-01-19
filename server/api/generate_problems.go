// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
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
	maxProbs int = 50
	// difficulty epsilon as a multiple
	problemSelectionEpsilon = 0.1
)

// Get all problems that satisfy this ProblemTypeBitmap and have similar Difficulty
func (a *Api) getSatisfyingProblems(logPrefix string, c *gin.Context, settings *Settings) (*[]Problem, error) {
	permutations := GetProblemTypePermutations(ProblemType(settings.ProblemTypeBitmap))
	diffLowerBound := settings.TargetDifficulty * (1 - problemSelectionEpsilon)
	diffUpperBound := settings.TargetDifficulty * (1 + problemSelectionEpsilon)
	sql := fmt.Sprintf(
		"SELECT * FROM problems WHERE problem_type_bitmap IN (%s) AND difficulty >= %g and difficulty <= %g AND disabled=0;",
		strings.Replace(strings.Trim(fmt.Sprint(permutations), "[]"), " ", ",", -1),
		diffLowerBound,
		diffUpperBound,
	)
	fmt.Printf("sql: %s\n", sql)
	problems, status, msg, err := a.problemManager.CustomList(sql)
	if HandleMngrResp(logPrefix, c, status, msg, err, problems) != nil {
		return nil, err
	}
	return problems, nil
}

func (a *Api) selectProblem(logPrefix string, c *gin.Context, settings *Settings) (*Problem, error) {
	problems, err := a.getSatisfyingProblems(logPrefix, c, settings)
	if err != nil {
		return nil, err
	}

	if len(*problems) >= maxProbs*2 {
		glog.Infof("%s returning random problem because there are already %d problems", logPrefix, len(*problems))
		return &(*problems)[rand.Intn(len(*problems))], nil
	}

	glog.Infof("%s generating new problems because there are only %d problems", logPrefix, len(*problems))

	go a.generateProblem(logPrefix, c, settings, maxProbs)
	return a.generateProblem(logPrefix, c, settings, 1)

}

func (a *Api) generateProblem(logPrefix string, c *gin.Context, settings *Settings, numProblems int) (*Problem, error) {
	var model *Problem
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
		uniqueIds[model.Id] = true

		// Write to database, checking for collisions
		_, status, _, err := a.problemManager.Get(model.Id)
		if err == nil {
			// already exists
			continue
		}
		if status == http.StatusInternalServerError {
			glog.Infof("%s generator error when checking for collisions: (%d: %s)", logPrefix, status, err)
		}
		newCount += 1
		status, msg, err := a.problemManager.Create(model)
		if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
			return nil, err
		}
	}
	glog.Infof("%s generator numProblems requested: %d vs unique problems generated: %d", logPrefix, numProblems, len(uniqueIds))
	glog.Infof("%s generator numProblems requested: %d vs new problems generated: %d", logPrefix, numProblems, newCount)

	// Just return the last problem added
	return model, nil
}
