package api

import (
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/mathcore"
)

// TestGenerationFunnel_NoSilentDrops: every reject lands in a named stage and
// the funnel line accounts for all of them.
func TestGenerationFunnel_NoSilentDrops(t *testing.T) {
	f := newGenerationFunnel(10)
	f.returned = 8
	f.reject(mathcore.RejectLexer)
	f.reject(rejectAnswer)
	f.reject(rejectAnswer)
	f.inserted = 5
	line := f.String()
	for _, want := range []string{"requested=10", "returned=8", "lexer=1", "answer=2", "inserted=5",
		"unknown_rules=0", "collision=0", "envelope=0", "validator=0", "create=0"} {
		if !strings.Contains(line, want) {
			t.Errorf("funnel line missing %q: %s", want, line)
		}
	}
}

// TestVerifyAnswer covers the exported answer check used by tooling: a form
// that evaluates to the answer passes; a wrong answer or an unlexable form
// fails.
func TestVerifyAnswer(t *testing.T) {
	if err := VerifyAnswer("9999 / 3 / 3", "1111"); err != nil {
		t.Errorf("valid form rejected: %v", err)
	}
	if err := VerifyAnswer("60 * 2", "120"); err != nil {
		t.Errorf("valid form rejected: %v", err)
	}
	if VerifyAnswer("60 * 2", "121") == nil {
		t.Error("wrong answer accepted")
	}
	if VerifyAnswer("2 ^ 3", "8") == nil {
		t.Error("unlexable form accepted")
	}
}
