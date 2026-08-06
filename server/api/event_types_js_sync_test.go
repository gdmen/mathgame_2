package api

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The client posts event types across a language boundary, so neither compiler
// can see a rename or a typo. TestEventTypesMatchJS closes that gap: the JS
// mirror must hold exactly the server's vocabulary, and every event type the
// client posts must come from that mirror rather than a literal.

const (
	jsEnumsPath = "../../web/src/enums.js"
	jsSrcDir    = "../../web/src"
)

var (
	// Anchored to declaration shape so a quoted word in a comment inside either
	// block cannot pass for an entry.
	goEventDecl = regexp.MustCompile(`(?m)^\s*\w+\s*=\s*"([^"]*)"`)
	jsEventDecl = regexp.MustCompile(`(?m)^\s*\w+:\s*"([^"]*)"`)

	quotedString = regexp.MustCompile(`"([^"]*)"`)
	// The event-posting call sites, matched by position: a misspelled event type
	// equals no known constant, so value-based matching alone would miss it.
	eventCallSite = regexp.MustCompile(`(?:\bpost(?:Event)?|\.add|\.remove)\(\s*"([^"]*)"`)
)

// declaredValues returns the values declared by decl between the start and end
// markers of the file at path.
func declaredValues(t *testing.T, path, start, end string, decl *regexp.Regexp) []string {
	t.Helper()
	src := readFileForTest(t, path)
	i := strings.Index(src, start)
	if i < 0 {
		t.Fatalf("%s: marker %q not found", path, start)
	}
	i += len(start)
	j := strings.Index(src[i:], end)
	if j < 0 {
		t.Fatalf("%s: marker %q not found after %q", path, end, start)
	}
	var out []string
	for _, m := range decl.FindAllStringSubmatch(src[i:i+j], -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("%s: no values parsed between %q and %q", path, start, end)
	}
	sort.Strings(out)
	return out
}

func TestEventTypesMatchJS(t *testing.T) {
	goTypes := declaredValues(t, "event_types.go", "// EventTypes", "// -end- EventTypes", goEventDecl)
	jsTypes := declaredValues(t, jsEnumsPath, "const EventTypes = {", "};", jsEventDecl)

	if strings.Join(goTypes, ",") != strings.Join(jsTypes, ",") {
		t.Errorf("event types differ:\n  event_types.go: %v\n  %s: %v", goTypes, jsEnumsPath, jsTypes)
	}

	known := map[string]bool{}
	for _, et := range goTypes {
		known[et] = true
	}

	err := filepath.WalkDir(jsSrcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		// Jest suites are exempt: asserting on the wire string is the point there.
		if d.IsDir() || !strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".test.js") ||
			path == filepath.Clean(jsEnumsPath) {
			return nil
		}
		src := readFileForTest(t, path)
		for _, m := range quotedString.FindAllStringSubmatch(src, -1) {
			if known[m[1]] {
				t.Errorf("%s: bare event-type literal %q - use EventTypes from enums.js", path, m[1])
			}
		}
		for _, m := range eventCallSite.FindAllStringSubmatch(src, -1) {
			if !known[m[1]] {
				t.Errorf("%s: literal %q posted as an event type - use EventTypes from enums.js", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", jsSrcDir, err)
	}
}
