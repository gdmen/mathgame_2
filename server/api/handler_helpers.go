// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

func GetAuth0IdFromContext(c *gin.Context) string {
	return c.MustGet(common.Auth0IdKey).(string)
}

func GetUserFromContext(c *gin.Context) *User {
	return c.MustGet(common.UserKey).(*User)
}

func BindModelFromForm(logPrefix string, c *gin.Context, model interface{}) error {
	err := c.ShouldBindJSON(model)
	if err != nil {
		msg := "Couldn't parse input JSON body"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, common.GetError(msg))
		return err
	}
	glog.Infof("%s %s", logPrefix, model)
	return nil
}

func BindModelFromURI(logPrefix string, c *gin.Context, model interface{}) error {
	err := c.ShouldBindUri(model)
	if err != nil {
		msg := "Couldn't parse URI"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, common.GetError(msg))
		return err
	}
	glog.Infof("%s %s", logPrefix, model)
	return nil
}

func HandleMngrResp(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}) error {
	return OptionalWriteHandleMngrResp(logPrefix, c, status, msg, err, model, false)
}

func HandleMngrRespWriteCtx(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}) error {
	return OptionalWriteHandleMngrResp(logPrefix, c, status, msg, err, model, true)
}

func OptionalWriteHandleMngrResp(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}, writeCtx bool) error {
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, common.GetError(msg))
		return err
	}

	glog.Infof("%s (HTTP %d) %T: %+v", logPrefix, status, model, model)
	if writeCtx {
		glog.Infof("%s writing to reponse body: %v", logPrefix, model)
		c.JSON(status, model)
	}
	return nil
}
