package common

import (
	"github.com/gin-gonic/gin"
	"github.com/satori/go.uuid"
)

const (
	RequestIdKey = "X-Request-Id"
	Auth0IdKey   = "X-Auth0-Id"
)

func RequestIdMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := uuid.NewV4().String()
		c.Set(RequestIdKey, rid)
		c.Header(RequestIdKey, rid)
	}
}

type TestAuth0 struct {
	Auth0Id string `json:"test_auth0_id" uri:"test_auth0_id" form:"test_auth0_id"`
}

func TestAuth0Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		model := &TestAuth0{}
		err := c.Bind(model)
		if err != nil {
			panic(err)
		}
		c.Set(Auth0IdKey, model.Auth0Id)
	}
}
