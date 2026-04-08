package api

import (
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

func TestChooseWeightedTopic_WeakTopicWeighted(t *testing.T) {
	stats := map[uint64]*TopicStat{
		uint64(ADDITION): {
			UserID: 1, ProblemType: uint64(ADDITION),
			Attempts: 20, Correct: 18, // 90% accuracy = strong
			TargetDifficulty: 5,
		},
		uint64(MULTIPLICATION): {
			UserID: 1, ProblemType: uint64(MULTIPLICATION),
			Attempts: 15, Correct: 6, // 40% accuracy = weak
			TargetDifficulty: 4,
		},
	}
	enabledBitmap := uint64(ADDITION | MULTIPLICATION)

	// Run many iterations to check the distribution
	counts := map[uint64]int{}
	n := 10000
	for i := 0; i < n; i++ {
		topic, _ := chooseWeightedTopic(stats, enabledBitmap, 3, func(max int) int {
			return i % max
		})
		counts[topic]++
	}

	// Multiplication (weak, weight=2) should appear ~2x as often as addition (weight=1)
	addCount := counts[uint64(ADDITION)]
	mulCount := counts[uint64(MULTIPLICATION)]
	ratio := float64(mulCount) / float64(addCount)
	if ratio < 1.5 || ratio > 2.5 {
		t.Errorf("expected weak topic ~2x more frequent, got addition=%d multiplication=%d ratio=%.2f", addCount, mulCount, ratio)
	}
}

func TestChooseWeightedTopic_NoStats(t *testing.T) {
	stats := map[uint64]*TopicStat{}
	enabledBitmap := uint64(ADDITION | SUBTRACTION)

	topic, diff := chooseWeightedTopic(stats, enabledBitmap, 5.0, func(max int) int { return 0 })
	if topic == 0 {
		t.Error("expected a topic even with no stats")
	}
	if diff != 5.0 {
		t.Errorf("expected base difficulty 5.0, got %f", diff)
	}
}

func TestChooseWeightedTopic_InsufficientAttempts(t *testing.T) {
	stats := map[uint64]*TopicStat{
		uint64(ADDITION): {
			UserID: 1, ProblemType: uint64(ADDITION),
			Attempts: 5, Correct: 1, // Low accuracy but < 10 attempts: not weighted
			TargetDifficulty: 3,
		},
		uint64(SUBTRACTION): {
			UserID: 1, ProblemType: uint64(SUBTRACTION),
			Attempts: 5, Correct: 4,
			TargetDifficulty: 3,
		},
	}
	enabledBitmap := uint64(ADDITION | SUBTRACTION)

	// Both should have weight=1 (insufficient attempts for weak weighting)
	counts := map[uint64]int{}
	n := 10000
	for i := 0; i < n; i++ {
		topic, _ := chooseWeightedTopic(stats, enabledBitmap, 3, func(max int) int { return i % max })
		counts[topic]++
	}

	addCount := counts[uint64(ADDITION)]
	subCount := counts[uint64(SUBTRACTION)]
	ratio := float64(addCount) / float64(subCount)
	if ratio < 0.8 || ratio > 1.2 {
		t.Errorf("expected roughly equal selection with insufficient attempts, got addition=%d subtraction=%d ratio=%.2f", addCount, subCount, ratio)
	}
}

func TestTopicStat_Accuracy(t *testing.T) {
	ts := TopicStat{Attempts: 20, Correct: 15}
	if ts.Accuracy() != 0.75 {
		t.Errorf("expected 0.75, got %f", ts.Accuracy())
	}
	ts2 := TopicStat{Attempts: 0, Correct: 0}
	if ts2.Accuracy() != 0 {
		t.Errorf("expected 0 for zero attempts, got %f", ts2.Accuracy())
	}
}

func TestRecordTopicAttempt_Integration(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|topic-test", "topic@test.com", "topictest")

	// Record some attempts
	bitmap := uint64(ADDITION | MULTIPLICATION) // Both topics set
	api.recordTopicAttempt("[test]", user.Id, bitmap, true, 3.0)
	api.recordTopicAttempt("[test]", user.Id, bitmap, false, 3.0)
	api.recordTopicAttempt("[test]", user.Id, uint64(ADDITION), true, 3.0) // Addition only

	stats, err := api.getTopicStats(user.Id)
	if err != nil {
		t.Fatalf("getTopicStats: %v", err)
	}

	// Addition: 3 attempts (2 from bitmap + 1 solo), 2 correct
	addStat := stats[uint64(ADDITION)]
	if addStat == nil {
		t.Fatal("expected addition stats")
	}
	if addStat.Attempts != 3 || addStat.Correct != 2 {
		t.Errorf("addition: expected 3 attempts, 2 correct; got %d attempts, %d correct", addStat.Attempts, addStat.Correct)
	}

	// Multiplication: 2 attempts (from bitmap), 1 correct
	mulStat := stats[uint64(MULTIPLICATION)]
	if mulStat == nil {
		t.Fatal("expected multiplication stats")
	}
	if mulStat.Attempts != 2 || mulStat.Correct != 1 {
		t.Errorf("multiplication: expected 2 attempts, 1 correct; got %d attempts, %d correct", mulStat.Attempts, mulStat.Correct)
	}
}

func TestAdjustTopicDifficulty_Integration(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	user := createTestUser(t, r, "auth0id|adjust-test", "adjust@test.com", "adjusttest")

	// createTestUser inits topic_stats at default difficulty (3.0).
	// Set difficulty to 5.0 explicitly for this test.
	_, err = api.DB.Exec(`UPDATE topic_stats SET target_difficulty = 5.0 WHERE user_id = ?`, user.Id)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate 15 attempts, 14 correct for addition (93% accuracy)
	for i := 0; i < 14; i++ {
		api.recordTopicAttempt("[test]", user.Id, uint64(ADDITION), true, 5.0)
	}
	api.recordTopicAttempt("[test]", user.Id, uint64(ADDITION), false, 5.0)

	// Simulate 12 attempts, 4 correct for multiplication (33% accuracy)
	for i := 0; i < 4; i++ {
		api.recordTopicAttempt("[test]", user.Id, uint64(MULTIPLICATION), true, 5.0)
	}
	for i := 0; i < 8; i++ {
		api.recordTopicAttempt("[test]", user.Id, uint64(MULTIPLICATION), false, 5.0)
	}

	stats, _ := api.getTopicStats(user.Id)
	api.adjustTopicDifficulty("[test]", user.Id, stats)

	// Re-read stats after adjustment
	stats, _ = api.getTopicStats(user.Id)

	addStat := stats[uint64(ADDITION)]
	if addStat == nil {
		t.Fatal("expected addition stats after adjustment")
	}
	// Addition was 93% accuracy with difficulty 5.0 → should increase
	if addStat.TargetDifficulty <= 5.0 {
		t.Errorf("expected addition difficulty to increase from 5.0, got %f", addStat.TargetDifficulty)
	}
	// Attempts should be reset
	if addStat.Attempts != 0 {
		t.Errorf("expected attempts reset to 0 after adjustment, got %d", addStat.Attempts)
	}

	mulStat := stats[uint64(MULTIPLICATION)]
	if mulStat == nil {
		t.Fatal("expected multiplication stats after adjustment")
	}
	// Multiplication was 33% accuracy with difficulty 5.0 → should decrease
	if mulStat.TargetDifficulty >= 5.0 {
		t.Errorf("expected multiplication difficulty to decrease from 5.0, got %f", mulStat.TargetDifficulty)
	}
}
