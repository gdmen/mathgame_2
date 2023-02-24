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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.userManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateUser(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &User{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.userManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.problemManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Problem{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.problemManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.videoManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateVideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Video{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.videoManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
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
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Settings{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.settingsManager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) getSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Settings{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.settingsManager.Get(model.UserId)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.settingsManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Settings{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.settingsManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteSettings(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Settings{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.settingsManager.Delete(model.UserId)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Gamestate{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.gamestateManager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) getGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Gamestate{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.gamestateManager.Get(model.UserId)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.gamestateManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Gamestate{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.gamestateManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteGamestate(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Gamestate{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.gamestateManager.Delete(model.UserId)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Event{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.eventManager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) getEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Event{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.eventManager.Get(model.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.eventManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Event{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.eventManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Event{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.eventManager.Delete(model.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}

func (a *Api) createUserhasvideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Userhasvideo{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.userHasVideoManager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) getUserhasvideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Userhasvideo{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Read from database
	model, status, msg, err := a.userHasVideoManager.Get(model.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) listUserhasvideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.userHasVideoManager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {
		return
	}
}

func (a *Api) updateUserhasvideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Userhasvideo{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.userHasVideoManager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) deleteUserhasvideo(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Userhasvideo{}
	if BindModelFromURI(logPrefix, c, model) != nil {
		return
	}

	// Write to database
	status, msg, err := a.userHasVideoManager.Delete(model.Id)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {
		return
	}
}
