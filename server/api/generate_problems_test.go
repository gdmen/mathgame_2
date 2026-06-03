package api

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGenerateProblemsBackground_DedupPerUser: 10 concurrent calls for one user run exactly once.
func TestGenerateProblemsBackground_DedupPerUser(t *testing.T) {
	var (
		inFlight      atomic.Int32
		maxConcurrent atomic.Int32
		totalCalls    atomic.Int32
		startedAll    = make(chan struct{})
	)

	originalFn := backgroundGenFn
	defer func() { backgroundGenFn = originalFn }()
	defer backgroundGenLocks.Delete(uint32(42))

	backgroundGenFn = func(a *Api, logPrefix string, settings *Settings, numProblems int) {
		totalCalls.Add(1)
		cur := inFlight.Add(1)
		for {
			prev := maxConcurrent.Load()
			if cur <= prev || maxConcurrent.CompareAndSwap(prev, cur) {
				break
			}
		}
		<-startedAll
		time.Sleep(50 * time.Millisecond)
		inFlight.Add(-1)
	}

	api := &Api{}
	settings := &Settings{UserId: 42}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = api.generateProblemsBackground("[test-dedup]", settings)
		}()
	}
	wg.Wait()
	close(startedAll)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && inFlight.Load() > 0 {
		time.Sleep(10 * time.Millisecond)
	}

	if got := maxConcurrent.Load(); got != 1 {
		t.Errorf("maxConcurrent = %d, want 1", got)
	}
	if got := totalCalls.Load(); got != 1 {
		t.Errorf("totalCalls = %d, want 1", got)
	}
}

// TestGenerateProblemsBackground_DedupIsPerUser: two users run concurrently (peak in-flight == 2).
func TestGenerateProblemsBackground_DedupIsPerUser(t *testing.T) {
	var (
		inFlight      atomic.Int32
		maxConcurrent atomic.Int32
	)
	originalFn := backgroundGenFn
	defer func() { backgroundGenFn = originalFn }()
	defer backgroundGenLocks.Delete(uint32(101))
	defer backgroundGenLocks.Delete(uint32(102))

	done := make(chan struct{})
	backgroundGenFn = func(a *Api, logPrefix string, settings *Settings, numProblems int) {
		cur := inFlight.Add(1)
		for {
			prev := maxConcurrent.Load()
			if cur <= prev || maxConcurrent.CompareAndSwap(prev, cur) {
				break
			}
		}
		<-done
		inFlight.Add(-1)
	}

	api := &Api{}
	_ = api.generateProblemsBackground("[test]", &Settings{UserId: 101})
	_ = api.generateProblemsBackground("[test]", &Settings{UserId: 102})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && inFlight.Load() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	close(done)

	if got := maxConcurrent.Load(); got != 2 {
		t.Errorf("maxConcurrent = %d, want 2", got)
	}
}
