// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/generator"
)

const (
	// EventTypes
	LOGGED_IN           = "logged_in"           // no value
	DISPLAYED_PROBLEM   = "displayed_problem"   // int64 ProblemID
	WORKING_ON_PROBLEM  = "working_on_problem"  // int Duration in seconds
	ANSWERED_PROBLEM    = "answered_problem"    // string Answer
	WATCHING_VIDEO      = "watching_video"      // int Duration in seconds
	DONE_WATCHING_VIDEO = "done_watching_video" // int64 VideoID
	// -end- EventTypes
)

var EventTypes = [...]string{
	LOGGED_IN,
	DISPLAYED_PROBLEM,
	WORKING_ON_PROBLEM,
	ANSWERED_PROBLEM,
	WATCHING_VIDEO,
	DONE_WATCHING_VIDEO,
}

// Do stuff based on the event and write a Gamestate{} to the context.
func (a *Api) processEvent(logPrefix string, c *gin.Context, event_type string) error {
	if event_type == LOGGED_IN {
	} else if event_type == DISPLAYED_PROBLEM {
	} else if event_type == WORKING_ON_PROBLEM {
	} else if event_type == ANSWERED_PROBLEM {
	} else if event_type == WATCHING_VIDEO {
	} else if event_type == DONE_WATCHING_VIDEO {
	} else {
		msg := fmt.Sprintf("Invalid EventType: %s", event_type)
		glog.Errorf("%s %s", logPrefix, msg)
		c.JSON(http.StatusBadRequest, msg)
		return errors.New(msg)
	}
	return nil
}

func (a *Api) customCreateEvent(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &Event{}
	if BindModelFromForm(logPrefix, c, model) != nil {
		return
	}

	// Validate EventType
	if a.processEvent(logPrefix, c, model.EventType) != nil {
		return
	}

	// Write to database
	status, msg, err := a.eventManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}

func (a *Api) customCreateProblem(c *gin.Context) {
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	opts := &generator.Options{}
	if BindModelFromURI(logPrefix, c, opts) != nil {
		return
	}
	if BindModelFromForm(logPrefix, c, opts) != nil {
		return
	}

	// Generate Problem
	model := &Problem{}
	// TODO: change this to a loop that tries to add problems until a new problem is added
	var err error
	model.Expression, model.Answer, model.Difficulty, err = generator.GenerateProblem(opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			glog.Errorf("%s %s: %v", logPrefix, msg, err)
			c.JSON(http.StatusBadRequest, GetError(msg))
			return
		}
		msg := "Couldn't generate problem"
		glog.Errorf("%s %s: %v", logPrefix, msg, err)
		c.JSON(http.StatusBadRequest, GetError(msg))
		return
	}

	// Use expression hash as model.Id
	h := fnv.New64a()
	h.Write([]byte(model.Expression))
	model.Id = h.Sum64()

	// Write to database
	status, msg, err := a.problemManager.Create(model)
	if HandleManagerResp(logPrefix, c, status, msg, err, model) != nil {
		return
	}
}
