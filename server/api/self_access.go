// Package api: the self-only access guard.
//
// A route whose path names a user may only be called by that user. The check
// lives in middleware rather than in each handler because the per-handler
// version was three forgettable lines and got forgotten: most read handlers
// bound the id straight out of the URI and served whatever it pointed at.
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/common"
)

// selfOnlyPathParams are the path params that name a user. RequireSelf compares
// each one it finds against the caller's identity; TestSelfOnly_* enumerates the
// router for them, so the route surface these tests cover is derived rather than
// hand-listed.
var selfOnlyPathParams = []string{"auth0_id", "user_id"}

// RequireSelf aborts the request with 403 unless every user-naming path param
// resolves to the caller. `auth0_id` is compared against the validated token's
// subject; `user_id` against the users row that UserMiddleware loaded from it,
// so RequireSelf must be registered after UserMiddleware. A route carrying
// neither param passes through untouched, which is what makes it safe to
// register on the whole authenticated route surface.
func (a *Api) RequireSelf() gin.HandlerFunc {
	return func(c *gin.Context) {
		if auth0Id, ok := c.Params.Get("auth0_id"); ok && auth0Id != GetAuth0IdFromContext(c) {
			abortNotSelf(c)
			return
		}
		if userId, ok := c.Params.Get("user_id"); ok {
			user := GetUserFromContextLenient(c)
			// An id that doesn't parse can't be the caller's either: fail closed
			// here instead of leaving it to the handler's own URI binding.
			id, err := strconv.ParseUint(userId, 10, 32)
			if err != nil || user == nil || uint32(id) != user.Id {
				abortNotSelf(c)
				return
			}
		}
		c.Next()
	}
}

// abortNotSelf answers without echoing whose id was asked for or whether it
// exists, so the response can't be used to probe for live accounts.
func abortNotSelf(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, common.GetError("Forbidden"))
}
