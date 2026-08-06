// Package llm_generator: retry helpers for transient OpenAI failures.
package llm_generator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang/glog"
	openai "github.com/sashabaranov/go-openai"
)

const (
	maxRetryAttempts = 4
	backoffFactor    = 2
)

// initialBackoff is a var so a test can drive the retry loop without sleeping.
var initialBackoff = 1 * time.Second

// chatCompletionWithRetry retries CreateChatCompletion with exponential backoff on transient errors.
func chatCompletionWithRetry(
	ctx context.Context,
	client *openai.Client,
	req openai.ChatCompletionRequest,
) (openai.ChatCompletionResponse, error) {
	var (
		resp    openai.ChatCompletionResponse
		err     error
		backoff = initialBackoff
	)
	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		resp, err = client.CreateChatCompletion(ctx, req)
		if err == nil {
			return resp, nil
		}
		if !isRetryableOpenAIError(err) {
			return resp, withOpenAIErrorCode(err)
		}
		if attempt == maxRetryAttempts {
			break
		}
		glog.Warningf("OpenAI transient error (attempt %d/%d, sleeping %s): %v",
			attempt, maxRetryAttempts, backoff, err)
		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= backoffFactor
	}
	return resp, withOpenAIErrorCode(err)
}

// withOpenAIErrorCode prefixes OpenAI's machine-readable error code onto the
// error the callers log: the client's Error() drops it, which leaves alerting
// (docs/ops-runbook.md) nothing but OpenAI's English prose to key on.
func withOpenAIErrorCode(err error) error {
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.Code == nil {
		return err
	}
	code := fmt.Sprintf("%v", apiErr.Code)
	if code == "" {
		return err
	}
	return fmt.Errorf("openai_code=%s: %w", code, err)
}

// isRetryableOpenAIError returns true for 408/429/5xx and unknown (likely network) errors.
func isRetryableOpenAIError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return isRetryableStatus(apiErr.HTTPStatusCode)
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return isRetryableStatus(reqErr.HTTPStatusCode)
	}
	// Unknown error: likely network/transport. Retry.
	return true
}

func isRetryableStatus(code int) bool {
	return code == 408 || code == 429 || (code >= 500 && code < 600)
}
