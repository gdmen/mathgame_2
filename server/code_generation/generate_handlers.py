# generate_handlers.py

import argparse
import os
import re

def get_comment_and_imports() -> str:
    s = '''// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	"garydmenezes.com/mathgame/server/common"
)
'''
    return s

def get_handler_string(m: dict) -> str:
    key_name = ""
    for f in m["fields"]:
        if "PRIMARY KEY" in f["sql"]:
            key_name = f["name"]
    s = '''
func (a *Api) create{0}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{0}{{}}
	if BindModelFromForm(logPrefix, c, model) != nil {{
		return
	}}

	// Write to database
	status, msg, err := a.{1}Manager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) get{0}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{0}{{}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

	// Read from database
	model, status, msg, err := a.{1}Manager.Get(model.{2})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) list{0}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Read from database
	models, status, msg, err := a.{1}Manager.List()
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {{
		return
	}}
}}

func (a *Api) update{0}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{0}{{}}
	if BindModelFromForm(logPrefix, c, model) != nil {{
		return
	}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

	// Write to database
	status, msg, err := a.{1}Manager.Update(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) delete{0}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{0}{{}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

	// Write to database
	status, msg, err := a.{1}Manager.Delete(model.{2})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {{
		return
	}}
}}
'''.format(
        m["name"].capitalize(),
        m["name"],
        key_name
    )

    return s

def main():
    parser = argparse.ArgumentParser(description="Generate API handlers for the math game.")
    parser.add_argument("-c", "--config", metavar="config", type=str, help="name of the config file (models.json)", required=True)
    parser.add_argument("-o", "--output", metavar="output", type=str, help="name of the output directory", required=True)
    args = parser.parse_args()
    c = {}
    with open(args.config, "r") as f:
        import json
        c = json.loads(f.read())
    with open(os.path.join(args.output,  "handlers.generated.go"), "w") as f:
        f.write(get_comment_and_imports())
        for m in c["models"]:
            f.write(get_handler_string(m))

if __name__ == "__main__":
    main()
