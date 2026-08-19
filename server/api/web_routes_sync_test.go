package api

import (
	"os"
	"regexp"
	"testing"
)

// TestWebRoutesNginxSync: every React route in the web/src/index.js Switch
// must be matched by a shell-route location regex in the nginx front door,
// or its URL answers with a 404 status on any hard load in production (the
// page still renders, so nothing visible flags it). The route list is
// otherwise guarded only by comments; this is the merge gate.
func TestWebRoutesNginxSync(t *testing.T) {
	indexJS, err := os.ReadFile("../../web/src/index.js")
	if err != nil {
		t.Fatalf("read index.js: %v", err)
	}
	conf, err := os.ReadFile("../../deploy/nginx/mikeymath.conf")
	if err != nil {
		t.Fatalf("read mikeymath.conf: %v", err)
	}

	// The shell-route locations: `location ~* ^/(...)...` with try_files
	// /app.html. Compile each pattern the same case-insensitive way nginx
	// applies ~*.
	locRe := regexp.MustCompile(`location ~\* (\S+) \{\s*\n\s*try_files /app\.html`)
	var shellRegexps []*regexp.Regexp
	for _, m := range locRe.FindAllStringSubmatch(string(conf), -1) {
		shellRegexps = append(shellRegexps, regexp.MustCompile("(?i)"+m[1]))
	}
	if len(shellRegexps) == 0 {
		t.Fatal("no shell-route locations found in mikeymath.conf; update this test alongside the config")
	}

	routeRe := regexp.MustCompile(`<Route exact path="([^"]+)"`)
	routes := routeRe.FindAllStringSubmatch(string(indexJS), -1)
	if len(routes) == 0 {
		t.Fatal("no routes found in web/src/index.js; update this test alongside it")
	}

	for _, m := range routes {
		route := m[1]
		if route == "/" {
			// The landing document, deliberately not the shell.
			continue
		}
		// A :param segment matches as any literal segment would.
		probe := regexp.MustCompile(`:[^/]+`).ReplaceAllString(route, "x")
		matched := false
		for _, re := range shellRegexps {
			if re.MatchString(probe) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("route %q (as %q) matches no shell-route location in deploy/nginx/mikeymath.conf; its URL will answer with a 404 status on hard load in production", route, probe)
		}
	}
}
