// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"hash/fnv"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/generator"
)

func (a *Api) generateProblem(logPrefix string, c *gin.Context, settings *Settings) (*Problem, error) {
	model := &Problem{}
	// TODO: this is temporary logic to make the generator compatible with the new ProblemTypeBitmap
	fractions := false //(FRACTIONS & settings.ProblemTypeBitmap) > 0
	negatives := false //(NEGATIVES & settings.ProblemTypeBitmap) > 0
	operations := []string{}
	if (ADDITION & settings.ProblemTypeBitmap) > 0 {
		operations = append(operations, "+")
	}
	if (SUBTRACTION & settings.ProblemTypeBitmap) > 0 {
		operations = append(operations, "-")
	}
	generator_opts := &generator.Options{
		Operations:       operations,
		Fractions:        fractions,
		Negatives:        negatives,
		TargetDifficulty: settings.TargetDifficulty,
	}

	var err error
	model.Expression, model.Answer, model.Difficulty, err = generator.GenerateProblem(generator_opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			c.JSON(http.StatusBadRequest, common.GetError(msg))
			return nil, err
		}
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, common.GetError(msg))
		return nil, err
	}
	model.ProblemTypeBitmap = 0
	if strings.Contains(model.Expression, "+") {
		model.ProblemTypeBitmap += ADDITION
	}
	if strings.Contains(model.Expression, "-") {
		model.ProblemTypeBitmap += SUBTRACTION
	}

	// Use expression hash as model.Id
	h := fnv.New32a()
	h.Write([]byte(model.Expression))
	model.Id = h.Sum32()

	// Write to database
	// TODO: collisions here will return the wrong Expre/Ans for the given problem id after returning a 200 for a duplicate?
	status, msg, err := a.problemManager.Create(model)
	if HandleMngrResp(logPrefix, c, status, msg, err, model) != nil {
		return nil, err
	}

	return model, nil
}
