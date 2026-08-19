// Package spec embeds the generated swagger.yaml so apiserver can serve it;
// see docs/swagger.md.
package spec

import _ "embed"

//go:embed swagger.yaml
var YAML []byte
