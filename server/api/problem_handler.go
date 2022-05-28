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

func (a *Api) createProblem(c *gin.Context) {
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
	manager := &ProblemManager{DB: a.DB}
	status, msg, err := manager.Create(model)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}

func (a *Api) deleteProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Problem{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	manager := &ProblemManager{DB: a.DB}
	status, msg, err := manager.Delete(model.Id)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success", logPrefix)
	c.JSON(status, nil)
	return
}

func (a *Api) getProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Problem{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	manager := &ProblemManager{DB: a.DB}
	model, status, msg, err := manager.Get(model.Id)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}

func (a *Api) listProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	manager := &ProblemManager{DB: a.DB}
	models, status, msg, err := manager.List()
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, models)
	c.JSON(status, models)
	return
}
