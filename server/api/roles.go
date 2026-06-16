// Package api: server-side authorization roles.
//
// The role lives on the users row (the DB is the source of truth) and defaults
// to RoleStudent. There is no seeding machinery: the single operator is
// promoted manually with `UPDATE users SET role='admin' WHERE auth0_id='...'`.
// Privileged surfaces gate on RequireAdmin.
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/common"
)

const (
	RoleStudent = "student"
	RoleAdmin   = "admin"
)

// RequireAdmin aborts the request with 403 unless the authenticated user has
// the admin role. It reads the user loaded by UserMiddleware (which resolves
// the validated token's identity to a users row), so it must be registered
// after UserMiddleware.
func (a *Api) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContextLenient(c)
		if user == nil || user.Role != RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, common.GetError("Admin access required."))
			return
		}
		c.Next()
	}
}

// adminWhoami confirms admin access works end to end and gives the
// /api/v1/admin group a first inhabitant for operator-only surfaces (e.g. a
// future admin-only calibration page) to register alongside.
func (a *Api) adminWhoami(c *gin.Context) {
	user := GetUserFromContext(c)
	c.JSON(http.StatusOK, gin.H{
		"auth0_id": user.Auth0Id,
		"id":       user.Id,
		"role":     user.Role,
	})
}
