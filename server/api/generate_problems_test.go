package api

import (
	"errors"
	"hash/fnv"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/llm_generator"
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

// llmTestProblem returns a canned LLM problem with sensible defaults for
// happy-path tests. Callers can mutate any field they care about.
func llmTestProblem() llm_generator.Problem {
	return llm_generator.Problem{
		Features:    []string{"addition"},
		Expression:  "12 + 7",
		Answer:      "19",
		Explanation: "12 + 7 = 19",
		Difficulty:  5,
	}
}

// withCannedLLM swaps llmGenerateProblemFn + llmValidateProblemFn for the
// duration of the test. Validation defaults to accept (returns nil); pass
// validateErr to simulate a rejection.
func withCannedLLM(t *testing.T, problems []llm_generator.Problem, genErr error, validateErr error) {
	t.Helper()
	originalGen := llmGenerateProblemFn
	originalValidate := llmValidateProblemFn
	llmGenerateProblemFn = func(opts *llm_generator.Options) ([]llm_generator.Problem, error) {
		if genErr != nil {
			return nil, genErr
		}
		return problems, nil
	}
	llmValidateProblemFn = func(p *llm_generator.Problem, gradeLevel int) error {
		return validateErr
	}
	t.Cleanup(func() {
		llmGenerateProblemFn = originalGen
		llmValidateProblemFn = originalValidate
	})
}

// TestGenerateProblems_LLM_HappyPath: a valid LLM problem flows through to
// a DB row with the right fields. Locks in the wire shape so any drift
// (e.g. dropped field on insert) shows up here.
func TestGenerateProblems_LLM_HappyPath(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	withCannedLLM(t, []llm_generator.Problem{llmTestProblem()}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(ADDITION),
		TargetDifficulty:  5,
		GradeLevel:        3,
	}
	problem, err := api.generateProblems("[test-llm-happy]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if problem == nil {
		t.Fatalf("expected a problem, got nil")
	}

	persisted, status, _, err := api.problemManager.Get(problem.Id)
	if err != nil || status != http.StatusOK {
		t.Fatalf("re-fetch id=%d: status=%d err=%v", problem.Id, status, err)
	}
	if persisted.Expression != "12 + 7" {
		t.Errorf("Expression = %q, want %q", persisted.Expression, "12 + 7")
	}
	if persisted.Answer != "19" {
		t.Errorf("Answer = %q, want %q", persisted.Answer, "19")
	}
	if persisted.Explanation != "12 + 7 = 19" {
		t.Errorf("Explanation = %q, want %q", persisted.Explanation, "12 + 7 = 19")
	}
	if persisted.Generator != llm_generator.VERSION {
		t.Errorf("Generator = %q, want %q", persisted.Generator, llm_generator.VERSION)
	}
	if persisted.ProblemTypeBitmap != uint64(ADDITION) {
		t.Errorf("ProblemTypeBitmap = %d, want %d", persisted.ProblemTypeBitmap, uint64(ADDITION))
	}
	if persisted.GradeLevel != 3 {
		t.Errorf("GradeLevel = %d, want 3", persisted.GradeLevel)
	}
	if persisted.DifficultyVersion != DifficultyVersion {
		t.Errorf("DifficultyVersion = %q, want %q", persisted.DifficultyVersion, DifficultyVersion)
	}
}

// TestGenerateProblems_LLM_IdCollisionSkipped: when the LLM returns a problem
// whose expression hashes to an already-occupied id, the loop must skip it
// (status != 404 from problemManager.Get) instead of overwriting.
func TestGenerateProblems_LLM_IdCollisionSkipped(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	canned := llmTestProblem()
	canned.Expression = "999 + 1"

	// Pre-seed a row with the same fnv32a(expression) so the next attempt
	// collides and gets dropped.
	h := fnv.New32a()
	h.Write([]byte(canned.Expression))
	existing := &Problem{
		Id:                h.Sum32(),
		ProblemTypeBitmap: uint64(ADDITION),
		Expression:        canned.Expression,
		Answer:            "1000",
		Difficulty:        5,
		Generator:         "test-seed",
		GradeLevel:        3,
	}
	if status, msg, err := api.problemManager.Create(existing); err != nil || status != http.StatusCreated {
		t.Fatalf("pre-seed: status=%d msg=%s err=%v", status, msg, err)
	}

	withCannedLLM(t, []llm_generator.Problem{canned}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(ADDITION),
		TargetDifficulty:  5,
		GradeLevel:        3,
	}
	_, err = api.generateProblems("[test-llm-collision]", settings, 1)
	// All canned problems collided, so no new problem produced -> error.
	if err == nil {
		t.Fatalf("expected error when all candidates collide, got nil")
	}

	// Seeded row should still carry its original Answer (not overwritten).
	persisted, status, _, err := api.problemManager.Get(existing.Id)
	if err != nil || status != http.StatusOK {
		t.Fatalf("re-fetch seeded id=%d: status=%d err=%v", existing.Id, status, err)
	}
	if persisted.Answer != "1000" {
		t.Errorf("collision overwrote seeded row; Answer = %q, want %q", persisted.Answer, "1000")
	}
}

// TestGenerateProblems_LLM_ValidationReject: a problem that fails
// llmValidateProblemFn must not be persisted.
func TestGenerateProblems_LLM_ValidationReject(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	canned := llmTestProblem()
	canned.Expression = "8 + 4"
	withCannedLLM(t, []llm_generator.Problem{canned}, nil, errors.New("rejected by test"))

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(ADDITION),
		TargetDifficulty:  5,
		GradeLevel:        3,
	}
	_, err = api.generateProblems("[test-llm-validate-reject]", settings, 1)
	if err == nil {
		t.Fatalf("expected error when sole candidate fails validation, got nil")
	}

	// Row should not exist in the DB.
	h := fnv.New32a()
	h.Write([]byte(canned.Expression))
	_, status, _, _ := api.problemManager.Get(h.Sum32())
	if status != http.StatusNotFound {
		t.Errorf("rejected problem was persisted; Get status = %d, want %d", status, http.StatusNotFound)
	}
}

// TestGenerateProblems_LLM_CalibrationReject: a problem whose self-reported
// difficulty diverges too far from target (ratio < 0.5 or > 2.0) must be
// dropped.
func TestGenerateProblems_LLM_CalibrationReject(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	canned := llmTestProblem()
	canned.Expression = "100 + 200"
	canned.Difficulty = 1 // target is 5, ratio 0.2 -> reject
	withCannedLLM(t, []llm_generator.Problem{canned}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(ADDITION),
		TargetDifficulty:  5,
		GradeLevel:        3,
	}
	_, err = api.generateProblems("[test-llm-calibration]", settings, 1)
	if err == nil {
		t.Fatalf("expected error when sole candidate fails calibration, got nil")
	}

	h := fnv.New32a()
	h.Write([]byte(canned.Expression))
	_, status, _, _ := api.problemManager.Get(h.Sum32())
	if status != http.StatusNotFound {
		t.Errorf("calibration-rejected problem was persisted; Get status = %d, want %d", status, http.StatusNotFound)
	}
}

// TestGenerateProblems_LLM_FallbackToHeuristic: when llmGenerateProblemFn
// returns an error, the path falls back to the heuristic generator (with
// WORD stripped) and produces a heuristic-generated problem instead.
func TestGenerateProblems_LLM_FallbackToHeuristic(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	withCannedLLM(t, nil, errors.New("OpenAI is on fire"), nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(ADDITION | WORD), // WORD will be stripped on fallback
		TargetDifficulty:  5,
		GradeLevel:        3,
	}
	problem, err := api.generateProblems("[test-llm-fallback]", settings, 1)
	if err != nil {
		t.Fatalf("expected fallback to produce a problem, got error: %v", err)
	}
	if problem == nil {
		t.Fatalf("fallback produced nil problem")
	}
	// Heuristic generator stamps its own version into Generator.
	if problem.Generator == llm_generator.VERSION {
		t.Errorf("fallback path produced an LLM-tagged problem; Generator = %q", problem.Generator)
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
		GradeLevel:       3,
	}
	problem, count, _ := api.runHeuristicGenerator("[test-difficulty-version]", settings, 1, ADDITION)
	if count == 0 || problem == nil {
		t.Fatalf("expected one heuristic problem, got count=%d problem=%v", count, problem)
	}
	if problem.DifficultyVersion != DifficultyVersion {
		t.Errorf("returned model: DifficultyVersion = %q, want %q", problem.DifficultyVersion, DifficultyVersion)
	}

	// Persisted row should also carry the stamp (catches a regression where
	// the field is set on the in-memory struct but lost on the way to INSERT).
	persisted, status, _, err := api.problemManager.Get(problem.Id)
	if err != nil || status != 200 {
		t.Fatalf("re-fetch problem id=%d: status=%d err=%v", problem.Id, status, err)
	}
	if persisted.DifficultyVersion != DifficultyVersion {
		t.Errorf("persisted row: DifficultyVersion = %q, want %q", persisted.DifficultyVersion, DifficultyVersion)
	}
}
