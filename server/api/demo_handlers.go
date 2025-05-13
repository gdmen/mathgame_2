// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"math/rand"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (a *Api) demoStart(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Create a user with a new, unique DEMO id
	user := User{
		Auth0Id:  "DEMO|" + randSeq(64),
		Email:    "demo@mikeymath.org",
		Username: "Friend",
		Pin:      "1234",
	}
	a.saveUser(logPrefix, c, &user)

	// Respond with HTTP set cookie
}
