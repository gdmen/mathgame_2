package llm_generator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func TestIsRetryableOpenAIError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"api 408", &openai.APIError{HTTPStatusCode: 408}, true},
		{"api 429", &openai.APIError{HTTPStatusCode: 429}, true},
		{"api 500", &openai.APIError{HTTPStatusCode: 500}, true},
		{"api 503", &openai.APIError{HTTPStatusCode: 503}, true},
		{"api 400", &openai.APIError{HTTPStatusCode: 400}, false},
		{"api 401", &openai.APIError{HTTPStatusCode: 401}, false},
		{"api 404", &openai.APIError{HTTPStatusCode: 404}, false},
		{"req 429", &openai.RequestError{HTTPStatusCode: 429}, true},
		{"req 502", &openai.RequestError{HTTPStatusCode: 502}, true},
		{"req 400", &openai.RequestError{HTTPStatusCode: 400}, false},
		{"network error", errors.New("dial tcp: timeout"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryableOpenAIError(tc.err)
			if got != tc.want {
				t.Errorf("isRetryableOpenAIError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWithOpenAIErrorCode(t *testing.T) {
	quota := &openai.APIError{
		Code:           "insufficient_quota",
		HTTPStatusCode: 429,
		HTTPStatus:     "Too Many Requests",
		Message:        "You exceeded your current quota, please check your plan and billing details.",
	}
	cases := []struct {
		name       string
		err        error
		wantPrefix string
	}{
		{"quota", quota, "openai_code=insufficient_quota: "},
		{"wrapped quota", fmt.Errorf("narrate: %w", quota), "openai_code=insufficient_quota: "},
		{"non-string code", &openai.APIError{Code: 429, HTTPStatusCode: 429}, "openai_code=429: "},
		{"no code", &openai.APIError{HTTPStatusCode: 500}, ""},
		{"empty code", &openai.APIError{Code: "", HTTPStatusCode: 500}, ""},
		{"request error", &openai.RequestError{HTTPStatusCode: 502}, ""},
		{"network error", errors.New("dial tcp: timeout"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withOpenAIErrorCode(tc.err)
			if !strings.HasPrefix(got.Error(), tc.wantPrefix) {
				t.Errorf("withOpenAIErrorCode(%v) = %q, want prefix %q", tc.err, got, tc.wantPrefix)
			}
			if tc.wantPrefix == "" && got != tc.err {
				t.Errorf("withOpenAIErrorCode(%v) = %q, want the error unchanged", tc.err, got)
			}
			if !strings.Contains(got.Error(), tc.err.Error()) {
				t.Errorf("withOpenAIErrorCode(%v) = %q, dropped the original message", tc.err, got)
			}
			if !errors.Is(got, tc.err) {
				t.Errorf("withOpenAIErrorCode(%v) broke the error chain", tc.err)
			}
		})
	}
}

// stubOpenAI serves one canned error response for every chat completion call.
func stubOpenAI(t *testing.T, status int, code, message string) *openai.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":{"code":%q,"message":%q,"type":"invalid_request_error"}}`, code, message)
	}))
	t.Cleanup(srv.Close)
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = srv.URL
	return openai.NewClientWithConfig(cfg)
}

// The code has to survive the whole call, not just the helper: both of
// chatCompletionWithRetry's failure returns are what put it in the journal.
func TestChatCompletionWithRetryReportsErrorCode(t *testing.T) {
	restore := initialBackoff
	initialBackoff = 0
	t.Cleanup(func() { initialBackoff = restore })

	cases := []struct {
		name   string
		status int
		code   string
	}{
		{"non-retryable", http.StatusBadRequest, "invalid_api_key"},
		{"retries exhausted", http.StatusTooManyRequests, "insufficient_quota"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := stubOpenAI(t, tc.status, tc.code, "You exceeded your current quota.")
			_, err := chatCompletionWithRetry(context.Background(), client, openai.ChatCompletionRequest{})
			if err == nil {
				t.Fatal("chatCompletionWithRetry returned no error")
			}
			want := "openai_code=" + tc.code
			if !strings.Contains(err.Error(), want) {
				t.Errorf("chatCompletionWithRetry error = %q, want it to contain %q", err, want)
			}
		})
	}
}
