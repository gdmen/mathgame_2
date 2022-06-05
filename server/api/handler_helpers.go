// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"net/http"

	"github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

func GetAuth0IdFromContext(logPrefix string, c *gin.Context, isTest bool) string {
	if isTest {
		return c.MustGet(common.Auth0IdKey).(string)
	}

	claims := c.Request.Context().Value(jwtmiddleware.ContextKey{}).(*validator.ValidatedClaims)
	return claims.RegisteredClaims.Subject
}

func BindModelFromForm(logPrefix string, c *gin.Context, model interface{}) error {
	err := c.ShouldBindJSON(model)
	if err != nil {
		msg := "Couldn't parse input JSON body"
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

func HandleMngrResp(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}) error {
	return OptionalWriteHandleMngrResp(logPrefix, c, status, msg, err, model, false)
}

func HandleMngrRespWriteCtx(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}) error {
	return OptionalWriteHandleMngrResp(logPrefix, c, status, msg, err, model, true)
}

func OptionalWriteHandleMngrResp(logPrefix string, c *gin.Context, status int, msg string, err error, model interface{}, writeCtx bool) error {
	if err != nil {
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(status, GetError(msg))
		return err
	}

	glog.Infof("%s (HTTP %d) %T: %+v", logPrefix, status, model, model)
	if writeCtx {
		c.JSON(status, model)
	}
	return nil
}
