package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/docs/spec"
)

// adminSwaggerSpec serves the embedded spec to the admin API docs page.
func (a *Api) adminSwaggerSpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec.YAML)
}
