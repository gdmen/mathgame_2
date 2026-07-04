package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
	heuristic_generator "garydmenezes.com/mathgame/server/generator"
	"garydmenezes.com/mathgame/server/llm_generator"
	"garydmenezes.com/mathgame/server/mathcore"
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

// withEchoNarrator swaps the narration + validation seams for the duration of a
// test. The narrator wraps each heuristic-built skeleton's symbolic expression
// in trivial prose (a valid WORD problem), copying the skeleton's authoritative
// SymbolicExpression + Answer exactly as the real NarrateProblems does - so the
// math stays the heuristic's and only the prose is faked. Validation accepts
// (echoing the problem's features) unless validateErr is set; narrateErr
// simulates an OpenAI failure.
func withEchoNarrator(t *testing.T, narrateErr, validateErr error) {
	t.Helper()
	originalNarrate := llmNarrateProblemFn
	originalValidate := llmValidateProblemFn
	llmNarrateProblemFn = func(skeletons []llm_generator.Skeleton) ([]llm_generator.Problem, error) {
		if narrateErr != nil {
			return nil, narrateErr
		}
		out := make([]llm_generator.Problem, 0, len(skeletons))
		for _, s := range skeletons {
			out = append(out, llm_generator.Problem{
				Expression:         `\text{A word problem posing ` + s.SymbolicExpression + `.}`,
				SymbolicExpression: s.SymbolicExpression,
				Answer:             s.Answer,
				Explanation:        `\text{That is the computation.}`,
			})
		}
		return out, nil
	}
	llmValidateProblemFn = func(p *llm_generator.Problem) error {
		return validateErr
	}
	t.Cleanup(func() {
		llmNarrateProblemFn = originalNarrate
		llmValidateProblemFn = originalValidate
	})
}

// countByGenerator returns how many problems carry the given generator string.
func countByGenerator(t *testing.T, api *Api, generator string) int {
	t.Helper()
	var n int
	if err := api.DB.QueryRow(`SELECT COUNT(*) FROM problems WHERE generator = ?`, generator).Scan(&n); err != nil {
		t.Fatalf("count generator=%s: %v", generator, err)
	}
	return n
}

// TestGenerateProblems_NonWord_NotBatched: generateProblems is WORD-only - a
// pure non-WORD envelope batches nothing (non-WORD is generated live on the
// request path; runHeuristicGenerator itself is covered by
// TestRunHeuristicGenerator_StampsDifficultyVersion).
func TestGenerateProblems_NonWord_NotBatched(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.ADDITION | mathcore.MEDIUM_NUMBERS),
		TargetDifficulty:  6,
	}
	if got, err := api.generateProblems("[test-nonword]", settings, 5); err == nil {
		t.Fatalf("expected no batch generation for a non-WORD envelope, got %v", got)
	}
	if n := countByGenerator(t, api, heuristic_generator.VERSION); n != 0 {
		t.Errorf("non-WORD envelope batched %d heuristic rows, want 0 (non-WORD is live)", n)
	}
}

// TestGenerateProblems_Word_HappyPath: a WORD envelope builds a scored skeleton,
// narrates it, and stores an llm_0.6 row whose stored math is the skeleton's
// (SymbolicExpression answers correctly) and whose difficulty carries the word
// concept on top of the skeleton.
func TestGenerateProblems_Word_HappyPath(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	withEchoNarrator(t, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.SUBTRACTION | mathcore.MEDIUM_NUMBERS | mathcore.WORD),
		TargetDifficulty:  8,
	}
	// generateProblems returns the word problem when one is produced (word runs
	// after the non-word heuristic and overwrites the return value).
	problem, err := api.generateProblems("[test-word-happy]", settings, 3)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if problem.Generator != llm_generator.VERSION {
		t.Fatalf("Generator = %q, want %q (a narrated word problem)", problem.Generator, llm_generator.VERSION)
	}
	if problem.ProblemTypeBitmap&uint64(mathcore.WORD) == 0 {
		t.Errorf("bitmap %d missing WORD", problem.ProblemTypeBitmap)
	}
	if problem.ProblemTypeBitmap&^settings.ProblemTypeBitmap != 0 {
		t.Errorf("bitmap %d not a subset of envelope %d", problem.ProblemTypeBitmap, settings.ProblemTypeBitmap)
	}
	if problem.SymbolicExpression == "" {
		t.Fatalf("word problem stored no symbolic_expression")
	}
	if err := VerifyAnswer(problem.SymbolicExpression, problem.Answer); err != nil {
		t.Errorf("stored skeleton %q does not answer %q: %v", problem.SymbolicExpression, problem.Answer, err)
	}
	// Difficulty is scored from the skeleton with the word concept applied, so it
	// exceeds the bare skeleton's score.
	skeletonOnly := mathcore.ComputeProblemDifficulty(problem.SymbolicExpression, "")
	if problem.Difficulty <= skeletonOnly {
		t.Errorf("word difficulty %.2f should exceed the bare skeleton %.2f (word bonus)",
			problem.Difficulty, skeletonOnly)
	}
	if problem.DifficultyVersion != mathcore.DifficultyVersion {
		t.Errorf("DifficultyVersion = %q, want %q", problem.DifficultyVersion, mathcore.DifficultyVersion)
	}
}

// TestGenerateProblems_Word_ValidatorReject: a narration the secondary-model
// validator rejects is never stored, and (generateProblems being WORD-only)
// nothing else is produced, so it returns an error.
func TestGenerateProblems_Word_ValidatorReject(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	withEchoNarrator(t, nil, errors.New("validator says no"))

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.SUBTRACTION | mathcore.WORD),
		TargetDifficulty:  6,
	}
	if got, err := api.generateProblems("[test-word-validate-reject]", settings, 3); err == nil {
		t.Fatalf("expected an error when the sole narration is rejected, got %v", got)
	}
	if n := countByGenerator(t, api, llm_generator.VERSION); n != 0 {
		t.Errorf("validator-rejected narration was stored: %d %s rows", n, llm_generator.VERSION)
	}
}

// TestGenerateProblems_Word_NarrationFails: an OpenAI failure yields no WORD
// problem and no error-swallowing fallback (non-WORD is a selectProblem concern,
// not generateProblems'), so generateProblems returns an error.
func TestGenerateProblems_Word_NarrationFails(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	withEchoNarrator(t, errors.New("OpenAI is on fire"), nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.ADDITION | mathcore.WORD),
		TargetDifficulty:  6,
	}
	if got, err := api.generateProblems("[test-word-narration-fail]", settings, 3); err == nil {
		t.Fatalf("expected an error when narration fails, got %v", got)
	}
	if n := countByGenerator(t, api, llm_generator.VERSION); n != 0 {
		t.Errorf("narration failed but %d word rows landed", n)
	}
}

// TestGenerateProblems_WordOnly_NoCoreOp_Errors: a WORD-only bitmap has no
// non-WORD envelope to build a skeleton from, so nothing is generated (the
// settings rules forbid this bitmap; the generator degrades gracefully).
func TestGenerateProblems_WordOnly_NoCoreOp_Errors(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.WORD),
		TargetDifficulty:  6,
	}
	if got, err := api.generateProblems("[test-word-only]", settings, 3); err == nil {
		t.Fatalf("expected no problem for a WORD-only bitmap, got %v", got)
	}
}

// TestGenerateProblems_Word_CollisionDedup: two identical narrations hash to the
// same id; the second hits the collision stage so exactly one word row lands.
func TestGenerateProblems_Word_CollisionDedup(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	originalNarrate := llmNarrateProblemFn
	originalValidate := llmValidateProblemFn
	defer func() {
		llmNarrateProblemFn = originalNarrate
		llmValidateProblemFn = originalValidate
	}()
	fixed := llm_generator.Problem{
		Expression:         `\text{A word problem posing 9 - 4.}`,
		SymbolicExpression: "9 - 4",
		Answer:             "5",
		Explanation:        `\text{because 9 - 4 = 5}`,
	}
	llmNarrateProblemFn = func(skeletons []llm_generator.Skeleton) ([]llm_generator.Problem, error) {
		return []llm_generator.Problem{fixed, fixed}, nil
	}
	llmValidateProblemFn = func(p *llm_generator.Problem) error {
		return nil
	}

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.SUBTRACTION | mathcore.WORD),
		TargetDifficulty:  5,
	}
	if _, err := api.generateProblems("[test-word-collision]", settings, 2); err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if n := countByGenerator(t, api, llm_generator.VERSION); n != 1 {
		t.Errorf("duplicate narrations produced %d word rows, want 1", n)
	}
}

// TestRunHeuristicGenerator_StampsDifficultyVersion guards against future
// refactors silently dropping the version stamp in the heuristic insert
// path. Asserts both the returned model and the persisted DB row carry
// the current DifficultyVersion.
func TestRunHeuristicGenerator_StampsDifficultyVersion(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	settings := &Settings{
		UserId:           1,
		TargetDifficulty: 5,
	}
	problem := api.runHeuristicGenerator("[test-difficulty-version]", settings, 1, mathcore.ADDITION)
	if problem == nil {
		t.Fatalf("expected one heuristic problem, got nil")
	}
	if problem.DifficultyVersion != mathcore.DifficultyVersion {
		t.Errorf("returned model: DifficultyVersion = %q, want %q", problem.DifficultyVersion, mathcore.DifficultyVersion)
	}

	// Persisted row should also carry the stamp (catches a regression where
	// the field is set on the in-memory struct but lost on the way to INSERT).
	persisted, status, _, err := api.problemManager.Get(problem.Id)
	if err != nil || status != 200 {
		t.Fatalf("re-fetch problem id=%d: status=%d err=%v", problem.Id, status, err)
	}
	if persisted.DifficultyVersion != mathcore.DifficultyVersion {
		t.Errorf("persisted row: DifficultyVersion = %q, want %q", persisted.DifficultyVersion, mathcore.DifficultyVersion)
	}
}
