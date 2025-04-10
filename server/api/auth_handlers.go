// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

func (a *Api) authEmail(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := struct {
		Email string `json:"email"`
	}{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

}
