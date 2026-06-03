package llm_generator

import (
	"errors"
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
