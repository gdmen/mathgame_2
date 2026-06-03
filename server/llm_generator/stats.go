// Package llm_generator: LLM call-rate counters.
package llm_generator

import (
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

const llmStatsReportInterval = time.Hour

var (
	llmAttempts atomic.Int64
	llmFailures atomic.Int64
)

func init() {
	go reportLLMStats()
}

// reportLLMStats logs the LLM call rate every hour and resets counters.
func reportLLMStats() {
	t := time.NewTicker(llmStatsReportInterval)
	defer t.Stop()
	for range t.C {
		// Swap failures first to avoid a transient >100% rate from a racing call.
		failures := llmFailures.Swap(0)
		attempts := llmAttempts.Swap(0)
		if attempts == 0 {
			continue
		}
		rate := 100.0 * float64(failures) / float64(attempts)
		const msg = "OpenAI calls in last hour: %d attempts, %d failures (%.1f%%)"
		if rate >= 50 {
			glog.Warningf(msg, attempts, failures, rate)
		} else {
			glog.Infof(msg, attempts, failures, rate)
		}
	}
}

// recordLLMCall increments call counters for an OpenAI outcome.
func recordLLMCall(err error) {
	llmAttempts.Add(1)
	if err != nil {
		llmFailures.Add(1)
	}
}
