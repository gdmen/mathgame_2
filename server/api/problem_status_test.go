package api

import (
	"fmt"
	"testing"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

// TestSelectionExcludesNonActiveStatus: only status='active' rows are servable;
// deprecated/reported/incorrect are excluded from the candidate pool.
func TestSelectionExcludesNonActiveStatus(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	seed := func(id uint32, status string) {
		if _, err := api.DB.Exec(
			`INSERT INTO problems (id, problem_type_bitmap, expression, symbolic_expression, answer, difficulty, status, generator, difficulty_version)
			 VALUES (?, ?, 'seed', '', '1', 5, ?, 'test', '0.2')`,
			id, uint64(mathcore.ADDITION), status,
		); err != nil {
			t.Fatalf("seed %d: %v", id, err)
		}
	}
	seed(8001, StatusActive)
	seed(8002, StatusActive)
	seed(8003, StatusDeprecated)
	seed(8004, StatusReported)
	seed(8005, StatusIncorrect)

	settings := &Settings{UserId: 1, ProblemTypeBitmap: uint64(mathcore.ADDITION), TargetDifficulty: 5}
	prevIds := []uint32{}
	pids, err := api.getSatisfyingProblemIds("[test-status]", settings, &prevIds)
	if err != nil {
		t.Fatalf("getSatisfyingProblemIds: %v", err)
	}
	got := map[uint32]bool{}
	for _, id := range *pids {
		got[id] = true
	}
	if len(got) != 2 || !got[8001] || !got[8002] {
		t.Fatalf("served set = %v, want exactly the active rows {8001, 8002}", got)
	}
	for _, id := range []uint32{8003, 8004, 8005} {
		if got[id] {
			t.Errorf("non-active id %d was served", id)
		}
	}
}

// TestBadProblemEventsSetReported: both bad_problem_system and bad_problem_user
// mark the offending problem status='reported' (a single unvalidated-claim state).
func TestBadProblemEventsSetReported(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|bad-problem", "bad@test.com", "baduser")
	// The event flow assembles PlayData with a reward video at the end.
	insertVideosAndUserHasVideo(t, api, user.Id, 2)

	cases := []struct {
		eventType string
		id        uint32
	}{
		{BAD_PROBLEM_SYSTEM, 950001},
		{BAD_PROBLEM_USER, 950002},
	}
	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			prob := &Problem{
				Id:                tc.id,
				ProblemTypeBitmap: uint64(mathcore.ADDITION),
				Expression:        "1 + 1",
				Answer:            "2",
				Difficulty:        3,
				Generator:         "test",
			}
			if _, _, err := api.problemManager.Create(prob); err != nil {
				t.Fatalf("create problem: %v", err)
			}
			reportEvent(t, r, user, tc.eventType, fmt.Sprintf(`{"problem_id":%d}`, tc.id))

			got, _, _, err := api.problemManager.Get(tc.id)
			if err != nil {
				t.Fatalf("get problem: %v", err)
			}
			if got.Status != StatusReported {
				t.Errorf("%s: status = %q, want %q", tc.eventType, got.Status, StatusReported)
			}
		})
	}
}

// TestMigration46StatusBackfill runs the real migration 46 against a re-created
// pre-migration shape (status alongside the old disabled boolean) and checks the
// provenance-aware classification: event-flagged rows become 'reported' for BOTH
// historical value formats, no-event disabled rows and active old-generator rows
// become 'deprecated', disabled-provenance wins over the old-gen sweep, and a
// current-generator active row is untouched. Nothing produces 'incorrect'.
func TestMigration46StatusBackfill(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()

	// setupTestAPI already ran migration 46 (dropping disabled). Re-add it to
	// simulate a deployed DB about to be migrated, then re-run 46's backfill.
	if _, err := api.DB.Exec("ALTER TABLE problems ADD COLUMN disabled TINYINT NOT NULL DEFAULT 0"); err != nil {
		t.Fatalf("add disabled column: %v", err)
	}

	seedProblem := func(id uint32, gen string, disabled int) {
		if _, err := api.DB.Exec(
			`INSERT INTO problems (id, problem_type_bitmap, expression, answer, explanation, symbolic_expression, difficulty, status, generator, difficulty_version, disabled)
			 VALUES (?, ?, 'seed', '1', '', '', 5, 'active', ?, '0.2', ?)`,
			id, uint64(mathcore.ADDITION), gen, disabled,
		); err != nil {
			t.Fatalf("seed problem %d: %v", id, err)
		}
	}
	seedEvent := func(eventType, value string) {
		if _, err := api.DB.Exec(
			"INSERT INTO events (user_id, event_type, value) VALUES (1, ?, ?)",
			eventType, value,
		); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}

	// A: disabled + JSON-format system flag → reported.
	seedProblem(970001, "llm_0.3", 1)
	seedEvent(BAD_PROBLEM_SYSTEM, `{"problem_id":970001,"explanation":"bad"}`)
	// B: disabled + bare-numeric user flag → reported (legacy value format).
	seedProblem(970002, "llm_0.3", 1)
	seedEvent(BAD_PROBLEM_USER, "970002")
	// C: disabled, no event → deprecated (the migration-32 difficulty sweep).
	seedProblem(970003, "llm_0.3", 1)
	// D: active old-generator, no event → deprecated (old-generator sweep).
	seedProblem(970004, "heuristic_0.0", 0)
	// E: active current generator → stays active.
	seedProblem(970005, "heuristic_2.0", 0)
	// F: disabled old-gen + JSON flag → reported (disabled provenance wins).
	seedProblem(970006, "llm_0.1", 1)
	seedEvent(BAD_PROBLEM_SYSTEM, `{"problem_id":970006}`)
	// G: disabled + a malformed (non-numeric, non-JSON) event value → the
	// JSON_VALID guard yields NULL, matches no id, so it falls through to
	// deprecated instead of aborting the migration on invalid JSON.
	seedProblem(970007, "llm_0.3", 1)
	seedEvent(BAD_PROBLEM_USER, "undefined")

	body, err := migrationsFS.ReadFile("migrations/46.sql")
	if err != nil {
		t.Fatalf("read migration 46: %v", err)
	}
	if err := runOne(api.DB, string(body)); err != nil {
		t.Fatalf("run migration 46: %v", err)
	}

	want := map[uint32]string{
		970001: StatusReported,
		970002: StatusReported,
		970003: StatusDeprecated,
		970004: StatusDeprecated,
		970005: StatusActive,
		970006: StatusReported,
		970007: StatusDeprecated,
	}
	for id, expStatus := range want {
		p, _, _, err := api.problemManager.Get(id)
		if err != nil {
			t.Fatalf("get problem %d: %v", id, err)
		}
		if p.Status != expStatus {
			t.Errorf("problem %d: status = %q, want %q", id, p.Status, expStatus)
		}
	}
}
