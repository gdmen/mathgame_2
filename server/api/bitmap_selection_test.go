package api

import (
	"testing"

	"garydmenezes.com/mathgame/server/common"

	heuristic_generator "garydmenezes.com/mathgame/server/generator"
)

// TestBitwiseSubsetSelection_Semantics: the subset SQL returns exactly the
// rows whose bits are a subset of the enabled bitmap - equivalence check
// against a manual subset filter.
func TestBitwiseSubsetSelection_Semantics(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// Seed problems across bitmaps at difficulty 5.
	seed := []struct {
		id     uint32
		bitmap uint64
	}{
		{9001, uint64(ADDITION)},
		{9002, uint64(SUBTRACTION)},
		{9003, uint64(ADDITION | SUBTRACTION)},
		{9004, uint64(ADDITION | MEDIUM_NUMBERS)},
		{9005, uint64(ADDITION | WORD)},
		{9006, uint64(MULTIPLICATION)},
		{9007, uint64(ADDITION | SINGLE_VARIABLE)},
		{9008, 0}, // zero bitmap: must NEVER be served (subset of everything)
	}
	for _, s := range seed {
		if _, err := api.DB.Exec(
			`INSERT INTO problems (id, problem_type_bitmap, expression, symbolic_expression, answer, difficulty, disabled, generator, difficulty_version)
			 VALUES (?, ?, 'seed', '', '1', 5, 0, 'test', '0.2')`,
			s.id, s.bitmap,
		); err != nil {
			t.Fatalf("seed %d: %v", s.id, err)
		}
	}

	enabled := uint64(ADDITION | SUBTRACTION | MEDIUM_NUMBERS)
	settings := &Settings{UserId: 1, ProblemTypeBitmap: enabled, TargetDifficulty: 5}
	prevIds := []uint32{}
	pids, err := api.getSatisfyingProblemIds("[test-subset]", settings, &prevIds)
	if err != nil {
		t.Fatalf("getSatisfyingProblemIds: %v", err)
	}

	got := map[uint32]bool{}
	for _, id := range *pids {
		got[id] = true
	}
	for _, s := range seed {
		isSubset := s.bitmap != 0 && s.bitmap&^enabled == 0
		if got[s.id] != isSubset {
			t.Errorf("id=%d bitmap=%d: served=%v, want %v (manual subset filter)",
				s.id, s.bitmap, got[s.id], isSubset)
		}
	}

	// Topic-filtered variant: only rows containing SUBTRACTION.
	pids, err = api.getSatisfyingProblemIdsForTopic("[test-subset-topic]", settings, &prevIds, uint64(SUBTRACTION))
	if err != nil {
		t.Fatalf("getSatisfyingProblemIdsForTopic: %v", err)
	}
	got = map[uint32]bool{}
	for _, id := range *pids {
		got[id] = true
	}
	for _, s := range seed {
		want := s.bitmap != 0 && s.bitmap&^enabled == 0 && s.bitmap&uint64(SUBTRACTION) != 0
		if got[s.id] != want {
			t.Errorf("topic filter id=%d bitmap=%d: served=%v, want %v", s.id, s.bitmap, got[s.id], want)
		}
	}
}

// TestSelection_PrefersNewestGeneratorVersion: getSatisfyingProblemIds returns
// only the highest-ranked generator version present, and falls back to the
// next-highest version when the top tier is excluded. Seeds share one family so
// the assertions don't depend on the cross-family rank ordering.
func TestSelection_PrefersNewestGeneratorVersion(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// Same envelope (ADDITION) + difficulty 5, three llm versions.
	seed := []struct {
		id  uint32
		gen string
	}{
		{7001, "llm_0.1"},
		{7002, "llm_0.1"},
		{7003, "llm_0.2"},
		{7004, "llm_0.4"},
		{7005, "llm_0.4"},
		{7006, "llm_0.5"},
	}
	for _, s := range seed {
		if _, err := api.DB.Exec(
			`INSERT INTO problems (id, problem_type_bitmap, expression, symbolic_expression, answer, difficulty, disabled, generator, difficulty_version)
			 VALUES (?, ?, 'seed', '', '1', 5, 0, ?, '0.2')`,
			s.id, uint64(ADDITION), s.gen,
		); err != nil {
			t.Fatalf("seed %d: %v", s.id, err)
		}
	}

	settings := &Settings{UserId: 1, ProblemTypeBitmap: uint64(ADDITION), TargetDifficulty: 5}

	assertTier := func(label string, prevIds []uint32, want ...uint32) {
		t.Helper()
		pids, err := api.getSatisfyingProblemIds(label, settings, &prevIds)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		got := map[uint32]bool{}
		for _, id := range *pids {
			got[id] = true
		}
		if len(got) != len(want) {
			t.Fatalf("%s tier = %v, want %v", label, *pids, want)
		}
		for _, id := range want {
			if !got[id] {
				t.Errorf("%s tier missing id %d (got %v)", label, id, *pids)
			}
		}
	}

	// Newest tier only: the single llm_0.5 row (outranks llm_0.4).
	assertTier("[newest]", nil, 7006)
	// Exclude llm_0.5: falls back to the llm_0.4 tier.
	assertTier("[fallback]", []uint32{7006}, 7004, 7005)
	// Exclude llm_0.5 and llm_0.4: falls back to the next-highest (llm_0.2).
	assertTier("[fallback2]", []uint32{7006, 7004, 7005}, 7003)
	// Exclude through llm_0.2: falls back to the llm_0.1 tier.
	assertTier("[fallback3]", []uint32{7006, 7004, 7005, 7003}, 7001, 7002)
}

// TestWeightedTopicMaskGates: all three gate sites exclude magnitude bits.
func TestWeightedTopicMaskGates(t *testing.T) {
	// Gate 1: chooseWeightedTopic - magnitude-only bitmap yields no candidates.
	topic, _ := chooseWeightedTopic(map[uint64]*TopicStat{},
		uint64(MEDIUM_NUMBERS|LARGE_NUMBERS), 5.0, func(max int) int { return 0 }, nil)
	if topic != 0 {
		t.Errorf("magnitude-only bitmap chose topic %d, want 0", topic)
	}
	// Mixed bitmap never picks a magnitude bit.
	for i := 0; i < 50; i++ {
		i := i
		topic, _ := chooseWeightedTopic(map[uint64]*TopicStat{},
			uint64(ADDITION|MEDIUM_NUMBERS), 5.0, func(max int) int { return i % max }, nil)
		if topic == uint64(MEDIUM_NUMBERS) {
			t.Fatal("chooseWeightedTopic picked a magnitude bit")
		}
	}

	// Gates 2+3: recordTopicAttempt / initTopicStats never write magnitude rows.
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	api.recordTopicAttempt("[test-mask]", 1, uint64(ADDITION|MEDIUM_NUMBERS), true, 5.0)
	stats, err := api.getTopicStats(1)
	if err != nil {
		t.Fatalf("getTopicStats: %v", err)
	}
	if _, ok := stats[uint64(MEDIUM_NUMBERS)]; ok {
		t.Error("recordTopicAttempt wrote a magnitude-bit stats row")
	}
	if ts, ok := stats[uint64(ADDITION)]; !ok || ts.Attempts != 1 {
		t.Errorf("recordTopicAttempt should write exactly the ADDITION row: %+v", stats)
	}

	if err := api.initTopicStats(2, uint64(ADDITION|LARGE_NUMBERS|MEDIUM_NUMBERS), 5.0); err != nil {
		t.Fatalf("initTopicStats: %v", err)
	}
	stats, err = api.getTopicStats(2)
	if err != nil {
		t.Fatalf("getTopicStats: %v", err)
	}
	if len(stats) != 1 {
		t.Errorf("initTopicStats seeded %d rows, want 1 (ADDITION only): %+v", len(stats), stats)
	}
	if _, ok := stats[uint64(ADDITION)]; !ok {
		t.Error("initTopicStats missing the ADDITION row")
	}
}

// TestHeuristicFromBits_ChainedOff: with CHAINED_OPERATIONS disabled, the
// bit-driven generator never emits multi-operator expressions, and every
// number respects the magnitude bound, across many samples.
func TestHeuristicFromBits_ChainedOff(t *testing.T) {
	opts := &heuristic_generator.Options{
		Operations:    []string{"+", "-"},
		MaxOperand:    12,
		AllowMissing:  true,
		AllowMultiOp:  false,
		MaxChainLen:   MaxChainLen,
		SameDenomOnly: true,
	}
	for i := 0; i < 200; i++ {
		expr, answer, _, err := heuristic_generator.GenerateProblem(opts)
		if err != nil {
			t.Fatalf("GenerateProblem: %v", err)
		}
		f := parseProblemFeatures(expr)
		if f.numOps >= 2 {
			t.Fatalf("CHAINED off but got %d ops: %q", f.numOps, expr)
		}
		if f.maxMagnitude > 12 {
			t.Fatalf("MaxOperand 12 violated: %q (maxMagnitude %v)", expr, f.maxMagnitude)
		}
		toks, lexErr := LexExpression(NormalizeExpression(expr))
		if lexErr != nil {
			t.Fatalf("heuristic output doesn't lex: %q (%v)", expr, lexErr)
		}
		if err := verifyAnswerSymbolic(toks, answer); err != nil {
			t.Fatalf("heuristic answer fails evaluator: %q = %q (%v)", expr, answer, err)
		}
	}
}

// TestThinPoolBoost: thin pools boost, average pools don't, cap holds.
func TestThinPoolBoost(t *testing.T) {
	if b := thinPoolBoost(100, 100); b != 1.0 {
		t.Errorf("average pool boost = %v, want 1.0", b)
	}
	if b := thinPoolBoost(200, 100); b != 1.0 {
		t.Errorf("rich pool boost = %v, want 1.0", b)
	}
	if b := thinPoolBoost(50, 100); b != 2.0 {
		t.Errorf("half pool boost = %v, want 2.0", b)
	}
	if b := thinPoolBoost(1, 100); b != thinPoolBoostMax {
		t.Errorf("thin pool boost = %v, want cap %v", b, thinPoolBoostMax)
	}
	if b := thinPoolBoost(0, 100); b != thinPoolBoostMax {
		t.Errorf("empty pool boost = %v, want cap %v", b, thinPoolBoostMax)
	}
}
