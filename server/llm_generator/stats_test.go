package llm_generator

import (
	"errors"
	"testing"
)

func TestRecordLLMCall(t *testing.T) {
	// Reset to a clean baseline so this test is independent of any other
	// package-level activity (e.g., the init goroutine).
	llmAttempts.Store(0)
	llmFailures.Store(0)

	recordLLMCall(nil)
	recordLLMCall(nil)
	recordLLMCall(errors.New("boom"))

	if got := llmAttempts.Load(); got != 3 {
		t.Errorf("llmAttempts = %d, want 3", got)
	}
	if got := llmFailures.Load(); got != 1 {
		t.Errorf("llmFailures = %d, want 1", got)
	}
}
