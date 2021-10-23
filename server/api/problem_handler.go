// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"net/http"
	"strconv"

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
	err := c.Bind(opts)
	if err != nil {
		msg := "Couldn't parse input form"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return
	}
	glog.Infof("%s %s", logPrefix, opts)

	// Write to database
	manager := &ProblemManager{DB: a.DB}
	model, status, msg, err := manager.Create(opts)
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
	paramId, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		msg := "URL id should be an integer"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return
	}

	// Write to database
	manager := &ProblemManager{DB: a.DB}
	status, msg, err := manager.Delete(paramId)
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
	paramId, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		msg := "URL id should be an integer"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return
	}

	// Read from database
	manager := &ProblemManager{DB: a.DB}
	model, status, msg, err := manager.Get(paramId)
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
