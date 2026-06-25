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
// duration of the test. Validation defaults to accept (echoes the problem's
// own features); pass validateErr to simulate a WORD-validator rejection.
// Note: the validator seam is only consulted for WORD problems - symbolic
// canned problems are answer-checked by the in-process evaluator, so their
// Answer fields must actually be correct.
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
	llmValidateProblemFn = func(p *llm_generator.Problem, constraints string, featureNames []string) ([]string, error) {
		if validateErr != nil {
			return nil, validateErr
		}
		return p.Features, nil
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
		ProblemTypeBitmap: uint64(mathcore.ADDITION),
		TargetDifficulty:  5,
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
	if persisted.ProblemTypeBitmap != uint64(mathcore.ADDITION) {
		t.Errorf("ProblemTypeBitmap = %d, want %d", persisted.ProblemTypeBitmap, uint64(mathcore.ADDITION))
	}
	if persisted.DifficultyVersion != mathcore.DifficultyVersion {
		t.Errorf("DifficultyVersion = %q, want %q", persisted.DifficultyVersion, mathcore.DifficultyVersion)
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
		ProblemTypeBitmap: uint64(mathcore.ADDITION),
		Expression:        canned.Expression,
		Answer:            "1000",
		Difficulty:        5,
		Generator:         "test-seed",
	}
	if status, msg, err := api.problemManager.Create(existing); err != nil || status != http.StatusCreated {
		t.Fatalf("pre-seed: status=%d msg=%s err=%v", status, msg, err)
	}

	withCannedLLM(t, []llm_generator.Problem{canned}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.ADDITION),
		TargetDifficulty:  5,
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
		ProblemTypeBitmap: uint64(mathcore.ADDITION),
		TargetDifficulty:  5,
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
		ProblemTypeBitmap: uint64(mathcore.ADDITION),
		TargetDifficulty:  5,
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
		ProblemTypeBitmap: uint64(mathcore.ADDITION | mathcore.WORD),
		TargetDifficulty:  5,
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
	}
	problem, count, _ := api.runHeuristicGenerator("[test-difficulty-version]", settings, 1, mathcore.ADDITION)
	if count == 0 || problem == nil {
		t.Fatalf("expected one heuristic problem, got count=%d problem=%v", count, problem)
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

// TestGenerateProblems_WordPath: a WORD problem flows through the validator
// seam; validator-extracted topic features stamp the bitmap on top of the
// parser's shape bits.
func TestGenerateProblems_WordPath(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\text{Mia has 12 stickers. She gives away 5. How many are left?}`
	p.Answer = "7"
	p.Features = []string{"subtraction", "word"}
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.SUBTRACTION | mathcore.WORD),
		TargetDifficulty:  6,
	}
	problem, err := api.generateProblems("[test-word-path]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	want := uint64(mathcore.WORD | mathcore.SUBTRACTION)
	if problem.ProblemTypeBitmap != want {
		t.Errorf("bitmap = %d (%v), want %d (parser WORD + validator subtraction)",
			problem.ProblemTypeBitmap,
			mathcore.ProblemTypeToFeatures(mathcore.ProblemType(problem.ProblemTypeBitmap)), want)
	}
	if problem.Expression != p.Expression {
		t.Errorf("word expression mutated: %q", problem.Expression)
	}
}

// TestGenerateProblems_WordPath_SymbolicExpression: a word problem's
// symbolic_expression is validated and stored, its computation-shape bits are
// folded into the bitmap, and its difficulty is scored from the form (division
// on large, chained operands) rather than the prose (which would score as a
// small addition).
func TestGenerateProblems_WordPath_SymbolicExpression(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\text{There are 9999 beads shared equally among 3 jars, then each jar is split among 3 friends. How many beads per friend?}`
	p.SymbolicExpression = "9999 / 3 / 3"
	p.Answer = "1111"
	p.Features = []string{"division", "word"}
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.DIVISION | mathcore.WORD | mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS | mathcore.CHAINED_OPERATIONS),
		TargetDifficulty:  20,
	}
	problem, err := api.generateProblems("[test-word-symbolic]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}

	if problem.Expression != p.Expression {
		t.Errorf("word expression mutated: %q", problem.Expression)
	}
	if want := mathcore.AdmitExpression(p.SymbolicExpression).Expr; problem.SymbolicExpression != want {
		t.Errorf("symbolic_expression = %q, want %q", problem.SymbolicExpression, want)
	}
	// The form's shape bits are folded in alongside the validator's WORD/DIVISION.
	for _, bit := range []mathcore.ProblemType{mathcore.WORD, mathcore.DIVISION, mathcore.LARGE_NUMBERS, mathcore.CHAINED_OPERATIONS} {
		if problem.ProblemTypeBitmap&uint64(bit) == 0 {
			t.Errorf("bitmap %d (%v) missing %v from the symbolic form",
				problem.ProblemTypeBitmap, mathcore.ProblemTypeToFeatures(mathcore.ProblemType(problem.ProblemTypeBitmap)), bit)
		}
	}
	// Scored from the form (~21), not the prose (~12 as addition).
	if problem.Difficulty < 18 {
		t.Errorf("difficulty = %.2f, want >=18 (scored from the symbolic form)", problem.Difficulty)
	}
}

// TestGenerateProblems_WordProseOnlyFeatures: features expressed entirely in
// prose are invisible to the parser; the validator's reports stamp the bits
// so the corresponding toggles govern word problems at serve time. Each case
// also proves the envelope rejects the problem for a user missing the bit.
func TestGenerateProblems_WordProseOnlyFeatures(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	cases := []struct {
		name     string
		expr     string
		answer   string
		features []string
		want     uint64 // full expected bitmap incl. parser shape bits
		bit      uint64 // the prose-only bit under test
	}{
		{
			name:     "pure-prose algebra",
			expr:     `\text{Solve for x: 3x + 7 = 22}`,
			answer:   "5",
			features: []string{"single_variable", "word"},
			want:     uint64(mathcore.SINGLE_VARIABLE | mathcore.WORD | mathcore.MEDIUM_NUMBERS),
			bit:      uint64(mathcore.SINGLE_VARIABLE),
		},
		{
			name:     "prose mismatched denominators",
			expr:     `\text{Tom ate 1/2 of a pizza and Jane ate 1/3 of it. How much did they eat together?}`,
			answer:   "5/6",
			features: []string{"fractions", "mismatched_denominators", "addition", "word"},
			want:     uint64(mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS | mathcore.ADDITION | mathcore.WORD),
			bit:      uint64(mathcore.MISMATCHED_DENOMINATORS),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := llmTestProblem()
			p.Expression = tc.expr
			p.Answer = tc.answer
			p.Features = tc.features
			withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

			settings := &Settings{
				UserId:            1,
				ProblemTypeBitmap: tc.want,
				TargetDifficulty:  6,
			}
			problem, err := api.generateProblems("[test-prose-feature]", settings, 1)
			if err != nil {
				t.Fatalf("generateProblems: %v", err)
			}
			if problem.ProblemTypeBitmap != tc.want {
				t.Errorf("bitmap = %d (%v), want %d",
					problem.ProblemTypeBitmap,
					mathcore.ProblemTypeToFeatures(mathcore.ProblemType(problem.ProblemTypeBitmap)), tc.want)
			}

			// The same problem must NOT reach an envelope missing the bit.
			settings.ProblemTypeBitmap = tc.want &^ tc.bit
			if got, err := api.generateProblems("[test-prose-feature-reject]", settings, 1); err == nil {
				t.Fatalf("expected envelope reject without bit %d, got %v", tc.bit, got)
			}
		})
	}
}

// TestGenerateProblems_WordMultiStep: the validator reports
// chained_operations on multi-step prose (the parser cannot count
// operations inside \text{}), so the CHAINED bit stamps and the multi-step
// toggle governs word problems at serve time too.
func TestGenerateProblems_WordMultiStep(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\text{A garden has 5 rows of 12 plants. If 3 plants die, how many are left?}`
	p.Answer = "57"
	p.Features = []string{"multiplication", "subtraction", "chained_operations", "word"}
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.MULTIPLICATION | mathcore.SUBTRACTION | mathcore.CHAINED_OPERATIONS | mathcore.WORD),
		TargetDifficulty:  6,
	}
	problem, err := api.generateProblems("[test-word-multistep]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	want := uint64(mathcore.MULTIPLICATION | mathcore.SUBTRACTION | mathcore.CHAINED_OPERATIONS | mathcore.WORD)
	if problem.ProblemTypeBitmap != want {
		t.Errorf("bitmap = %d (%v), want %d (validator chained_operations must stamp)",
			problem.ProblemTypeBitmap,
			mathcore.ProblemTypeToFeatures(mathcore.ProblemType(problem.ProblemTypeBitmap)), want)
	}

	// The same problem must NOT reach a multi-step-off envelope.
	settings.ProblemTypeBitmap = uint64(mathcore.MULTIPLICATION | mathcore.SUBTRACTION | mathcore.WORD)
	if got, err := api.generateProblems("[test-word-multistep-reject]", settings, 1); err == nil {
		t.Fatalf("expected envelope reject for multi-step-off user, got %v", got)
	}
}

// TestGenerateProblems_WordMultiStep_ValidatorOmitsChained: the bug #246
// guards. The validator reports two core ops but OMITS chained_operations
// (its independent-checkbox failure mode); the stamp-time invariant must OR
// it in, so the row both stamps correctly and rejects for a multi-step-off
// user - even though the LLM never said "chained".
func TestGenerateProblems_WordMultiStep_ValidatorOmitsChained(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\text{A garden has 5 rows of 12 plants. If 3 plants die, how many are left?}`
	p.Answer = "57"
	p.Features = []string{"multiplication", "subtraction", "word"} // NOTE: no chained
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.MULTIPLICATION | mathcore.SUBTRACTION | mathcore.CHAINED_OPERATIONS | mathcore.WORD),
		TargetDifficulty:  6,
	}
	problem, err := api.generateProblems("[test-word-omits-chained]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if problem.ProblemTypeBitmap&uint64(mathcore.CHAINED_OPERATIONS) == 0 {
		t.Errorf("bitmap = %d (%v): invariant must OR in CHAINED despite validator omitting it",
			problem.ProblemTypeBitmap,
			mathcore.ProblemTypeToFeatures(mathcore.ProblemType(problem.ProblemTypeBitmap)))
	}

	// And it must reject for a multi-step-off user, even though the validator
	// never reported chained.
	settings.ProblemTypeBitmap = uint64(mathcore.MULTIPLICATION | mathcore.SUBTRACTION | mathcore.WORD)
	if got, err := api.generateProblems("[test-word-omits-chained-reject]", settings, 1); err == nil {
		t.Fatalf("expected envelope reject for multi-step-off user, got %v", got)
	}
}

// TestGenerateProblems_WordEnvelopeReject: validator-reported features
// outside the user's envelope reject the problem (the final subset check
// covers validator-stamped bits too).
func TestGenerateProblems_WordEnvelopeReject(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\text{A sale takes 0.5 off the price of 8 dollars. What do you pay?}`
	p.Answer = "4"
	p.Features = []string{"decimals", "multiplication", "word"}
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.MULTIPLICATION | mathcore.WORD),
		TargetDifficulty:  6,
	}
	if got, err := api.generateProblems("[test-word-envelope]", settings, 1); err == nil {
		t.Fatalf("expected envelope reject, got problem %v", got)
	}
}

// TestGenerateProblems_RewriteConsistency: a lone bare letter is rewritten to
// '?' in the stored expression AND in the explanation prose.
func TestGenerateProblems_RewriteConsistency(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = "12 - x = 5"
	p.Answer = "7"
	p.Explanation = `\text{x is 7 because 12 - 7 = 5}`
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.SUBTRACTION | mathcore.MISSING_NUMBER),
		TargetDifficulty:  6,
	}
	problem, err := api.generateProblems("[test-rewrite]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if problem.Expression != "12 - ? = 5" {
		t.Errorf("expression = %q, want rewritten form", problem.Expression)
	}
	if problem.Explanation != `\text{? is 7 because 12 - 7 = 5}` {
		t.Errorf("explanation letter not substituted: %q", problem.Explanation)
	}
	if problem.ProblemTypeBitmap != uint64(mathcore.SUBTRACTION|mathcore.MISSING_NUMBER) {
		t.Errorf("bitmap = %d", problem.ProblemTypeBitmap)
	}
}

// TestGenerateProblems_PreservesLatexNotation: storage keeps the original
// notation (KaTeX renders it); normalization is parsing-only.
func TestGenerateProblems_PreservesLatexNotation(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem()
	p.Expression = `\frac{1}{2} + \frac{1}{4}`
	p.Answer = "3/4"
	withCannedLLM(t, []llm_generator.Problem{p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.ADDITION | mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS),
		TargetDifficulty:  6,
	}
	problem, err := api.generateProblems("[test-latex]", settings, 1)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	if problem.Expression != p.Expression {
		t.Errorf("stored expression = %q, want original LaTeX preserved", problem.Expression)
	}
	want := uint64(mathcore.ADDITION | mathcore.FRACTIONS | mathcore.MISMATCHED_DENOMINATORS)
	if problem.ProblemTypeBitmap != want {
		t.Errorf("bitmap = %d, want %d", problem.ProblemTypeBitmap, want)
	}
}

// TestGenerateProblems_CollisionFunnel: a duplicate expression in the same
// batch hits the collision stage; exactly one row lands.
func TestGenerateProblems_CollisionFunnel(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	p := llmTestProblem() // "12 + 7" = "19"
	withCannedLLM(t, []llm_generator.Problem{p, p}, nil, nil)

	settings := &Settings{
		UserId:            1,
		ProblemTypeBitmap: uint64(mathcore.ADDITION | mathcore.MEDIUM_NUMBERS),
		TargetDifficulty:  5,
	}
	problem, err := api.generateProblems("[test-collision]", settings, 2)
	if err != nil {
		t.Fatalf("generateProblems: %v", err)
	}
	var n int
	if err := api.DB.QueryRow(`SELECT COUNT(*) FROM problems WHERE id = ?`, problem.Id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("duplicate batch produced %d rows, want 1", n)
	}
}
