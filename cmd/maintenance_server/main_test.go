package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandler: every path gets the page with a temporary-outage status.
func TestHandler(t *testing.T) {
	for _, path := range []string{"/", "/play", "/settings", "/api/v1/problems"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		Handler(w, req)
		resp := w.Result()
		if resp.StatusCode != 503 {
			t.Errorf("%s: status %d, want 503", path, resp.StatusCode)
		}
		if resp.Header.Get("Retry-After") == "" {
			t.Errorf("%s: missing Retry-After", path)
		}
		if !strings.Contains(w.Body.String(), "Mikey Math") {
			t.Errorf("%s: page body missing", path)
		}
	}
}
