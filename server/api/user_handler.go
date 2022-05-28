// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

func (a *Api) createUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &User{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	manager := &UserManager{DB: a.DB}
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

func (a *Api) updateUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &User{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	manager := &UserManager{DB: a.DB}
	status, msg, err := manager.Update(model)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}

func (a *Api) getUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &User{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	manager := &UserManager{DB: a.DB}
	model, status, msg, err := manager.Get(model.Auth0Id)
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return
}
