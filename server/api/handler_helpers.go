// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
)

func BindModelFromForm(logPrefix string, c *gin.Context, model interface{}) error {
	err := c.Bind(model)
	if err != nil {
		msg := "Couldn't parse input form"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
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
		c.JSON(http.StatusBadRequest, GetError(msg))
		return err
	}
	glog.Infof("%s %s", logPrefix, model)
	return nil
}

func HandleManagerResp(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}) error {
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return err
	}

	glog.Infof("%s Success: %+v", logPrefix, model)
	c.JSON(status, model)
	return nil
}
