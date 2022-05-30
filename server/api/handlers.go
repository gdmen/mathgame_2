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
	status, msg, err := a.userManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
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
	model, status, msg, err := a.userManager.Get(model.Auth0Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.userManager.List()
	if HandleManagerResp(logPrefix, c, status, msg, err, models) != nil {
		return
	}
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
	status, msg, err := a.userManager.Update(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &User{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.userManager.Delete(model.Auth0Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Problem{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.problemManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
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
	model, status, msg, err := a.problemManager.Get(model.Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.problemManager.List()
	if HandleManagerResp(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Problem{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.problemManager.Update(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
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
	status, msg, err := a.problemManager.Delete(model.Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.videoManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) getVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.videoManager.Get(model.Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.videoManager.List()
	if HandleManagerResp(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.videoManager.Update(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.videoManager.Delete(model.Id)
	if HandleManagerResp(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}
