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
    has_user_fk = False
    for f in m["fields"]:
        if "PRIMARY KEY" in f["sql"]:
            key_name = f["name"]
        if f["name"] == "UserId":
            has_user_fk = True
    has_additional_user_fk = has_user_fk and key_name != "UserId"

    model_name = m["name"].capitalize()
    manager_name = m["name"]
    get_user_line = "user := GetUserFromContext(c)" if has_additional_user_fk else ""
    user_id_arg = ", user.Id" if has_additional_user_fk else ""
    get_user_for_list = "user := GetUserFromContext(c)" if has_user_fk else ""
    list_arg = "user.Id" if has_user_fk else ""
    set_user_id_line = "model.UserId = GetUserFromContext(c).Id" if has_user_fk else ""

    s = f'''
func (a *Api) create{model_name}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{model_name}{{}}
	if BindModelFromForm(logPrefix, c, model) != nil {{
		return
	}}

    {set_user_id_line}

	// Write to database
	status, msg, err := a.{manager_name}Manager.Create(model)
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) get{model_name}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{model_name}{{}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

    {get_user_line}

	// Read from database
	model, status, msg, err := a.{manager_name}Manager.Get(model.{key_name}{user_id_arg})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) list{model_name}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

    {get_user_for_list}

	// Read from database
	models, status, msg, err := a.{manager_name}Manager.List({list_arg})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, models) != nil {{
		return
	}}
}}

func (a *Api) update{model_name}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{model_name}{{}}
	if BindModelFromForm(logPrefix, c, model) != nil {{
		return
	}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

    {get_user_line}
    {set_user_id_line}

	// Write to database
	status, msg, err := a.{manager_name}Manager.Update(model{user_id_arg})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, model) != nil {{
		return
	}}
}}

func (a *Api) delete{model_name}(c *gin.Context) {{
	logPrefix := common.GetLogPrefix(c)
	glog.Infof("%s fcn start", logPrefix)

	// Parse input
	model := &{model_name}{{}}
	if BindModelFromURI(logPrefix, c, model) != nil {{
		return
	}}

    {get_user_line}

	// Write to database
	status, msg, err := a.{manager_name}Manager.Delete(model.{key_name}{user_id_arg})
	if HandleMngrRespWriteCtx(logPrefix, c, status, msg, err, nil) != nil {{
		return
	}}
}}
'''
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
