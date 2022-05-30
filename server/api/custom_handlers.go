// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"hash/fnv"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/generator"
)

func (a *Api) customCreateProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	opts := &generator.Options{}
	if BindModelFromURI(logPrefix, c, opts) != nil {
		return
	}
	if BindModelFromForm(logPrefix, c, opts) != nil {
		return
	}

	// Generate Problem
	model := &Problem{}
	// TODO: change this to a loop that tries to add problems until a new problem is added
	var err error
	model.Expression, model.Answer, model.Difficulty, err = generator.GenerateProblem(opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			c.JSON(http.StatusBadRequest, GetError(msg))
			return
		}
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return
	}

	// Use expression hash as model.Id
	h := fnv.New64a()
	h.Write([]byte(model.Expression))
	model.Id = h.Sum64()

	// Write to database
	status, msg, err := a.problemManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}
