/*
Package classification Math Game API.

Documentation of our Math Game API.

  Schemes: http
  BasePath: /
  Version: 1.0.0

  Consumes:
  - application/json

  Produces:
  - application/json

  Security:
  - basic

  SecurityDefinitions:
  basic:
    type: basic

swagger:meta
*/
package docs

import "garydmenezes.com/mathgame/internal/api"

//swagger:response error
type ErrorResp struct {
	//in:body
	Body api.Error
}
