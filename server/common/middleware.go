package common

import (
	"net/http"

	"github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/validator"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/satori/go.uuid"
)

const (
	RequestIdKey = "X-Request-Id"
	Auth0IdKey   = "X-Auth0-Id"
	UserKey      = "X-User"
)

func RequestIdMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := uuid.NewV4().String()
		c.Set(RequestIdKey, rid)
		c.Header(RequestIdKey, rid)
	}
}

func Auth0IdMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.Request.Context().Value(jwtmiddleware.ContextKey{}).(*validator.ValidatedClaims)
		c.Set(Auth0IdKey, claims.RegisteredClaims.Subject)
	}
}

type TestAuth0 struct {
	Auth0Id string `json:"test_auth0_id" uri:"test_auth0_id" form:"test_auth0_id"`
}

func TestAuth0IdMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		logPrefix := GetLogPrefix(c)
		model := &TestAuth0{}
		err := c.Bind(model)
		if err != nil {
			glog.Errorf("%s error: %s", logPrefix, err)
			panic(err)
		}
		c.Set(Auth0IdKey, model.Auth0Id)
	}
}

type GetUserFn func(string, *gin.Context) (interface{}, error)

func UserMiddleware(getUser GetUserFn, strict bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		logPrefix := GetLogPrefix(c)
		user, err := getUser(logPrefix, c)
		if err != nil {
			msg := "Couldn't find a user associated with this token."
			glog.Errorf("%s %s: %s", logPrefix, msg, err)
			if strict {
				c.AbortWithStatusJSON(http.StatusNotFound, GetError(msg))
			}
		}
		c.Set(UserKey, user)
	}
}
