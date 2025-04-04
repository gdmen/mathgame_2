package auth

import (
	"net/http"
	"strings"
	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/common"
)

const (
	UserKey      = "X-User"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tStr := c.GetHeader("Authorization")
		if tStr == "" {
			msg := "Empty Authorization header."
			c.JSON(http.StatusUnauthorized, GetError(msg))
			c.Next()
		}

		parts := strings.Split(tStr, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			msg := "Invalid Authorization token."
			c.AbortWIthStatusJSON(http.StatusUnauthorized, GetError(msg))
		}

		claims, err := VerifyToken(parts[1])
		if err != nil {
			msg := "Invalid Authorization token."
			c.AbortWIthStatusJSON(http.StatusUnauthorized, GetError(msg))
		}

		c.Set(UserKey, claims["user_id"])
	}
}

// Modify the following
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
