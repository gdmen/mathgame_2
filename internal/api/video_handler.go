// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/internal/common"
)

func (a *Api) createVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	err := c.Bind(model)
	if err != nil {
		msg := "Couldn't parse input form"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": msg})
		return
	}
	glog.Infof("%s %s", logPrefix, model)

	// Write to database
	manager := &VideoManager{DB: a.DB}
	status, msg, err := manager.Create(model)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, gin.H{"message": msg})
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}

func (a *Api) updateVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	paramId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		msg := "URL id should be an integer"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusNotFound, gin.H{"message": msg})
		return
	}
	model := &Video{}
	err = c.Bind(model)
	if err != nil {
		msg := "Couldn't parse input form"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": msg})
		return
	}
	model.Id = paramId
	glog.Infof("%s %s", logPrefix, model)

	// Write to database
	manager := &VideoManager{DB: a.DB}
	status, msg, err := manager.Update(model)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, gin.H{"message": msg})
		return
	}
	glog.Infof("HERE: %s", msg)

	glog.Infof("%s Success: %+v", logPrefix, model)
	glog.Infof("STATUS: %d", status)
	c.JSON(status, model)
	return
}

func (a *Api) deleteVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	paramId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		msg := "URL id should be an integer"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, gin.H{"message": msg})
		return
	}

	// Write to database
	manager := &VideoManager{DB: a.DB}
	status, msg, err := manager.Delete(paramId)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, gin.H{"message": msg})
		return
	}

	glog.Infof("%s Success", logPrefix)
	c.JSON(status, nil)
	return
}

func (a *Api) getVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	paramId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		msg := "URL id should be an integer"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusNotFound, gin.H{"message": msg})
		return
	}

	// Read from database
	manager := &VideoManager{DB: a.DB}
	model, status, msg, err := manager.Get(paramId)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, gin.H{"message": msg})
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}

func (a *Api) listVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	manager := &VideoManager{DB: a.DB}
	models, status, msg, err := manager.List()
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, gin.H{"message": msg})
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, models)
	c.JSON(status, models)
	return
}
