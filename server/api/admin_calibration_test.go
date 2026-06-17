package api // import "garydmenezes.com/mathgame/server/api"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

// TestDifficultyCalibrationPage verifies the admin calibration endpoint: it is
// admin-gated, returns JSON, buckets live problems by stored difficulty
// (counting disabled rows separately), and surfaces each sampled expression,
// its generator version, and its difficulty-factor breakdown.
func TestDifficultyCalibrationPage(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	admin := createTestUser(t, r, "auth0id|calib-admin", "calib@test.com", "calibadmin")
	if _, err := api.DB.Exec("UPDATE users SET role=? WHERE auth0_id=?", RoleAdmin, admin.Auth0Id); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	seed := func(id int, expr string, diff float64, disabled int, gen string, bitmap uint64) {
		t.Helper()
		_, err := api.DB.Exec(
			"INSERT INTO problems (id, problem_type_bitmap, expression, answer, explanation, difficulty, disabled, generator, difficulty_version) VALUES (?,?,?,?,?,?,?,?,?)",
			id, bitmap, expr, "7", "", diff, disabled, gen, "0.2")
		if err != nil {
			t.Fatalf("seed problem %d: %v", id, err)
		}
	}
	// Two live + one disabled in bucket 7 ([6.5, 7.5)); one live in bucket 12.
	seed(111, "3 + 4", 7.2, 0, "llm_0.3", uint64(ADDITION))
	seed(222, `5 \times 6`, 7.4, 0, "heuristic_1.0", uint64(MULTIPLICATION))
	seed(333, "9 - 1", 7.1, 1, "llm_0.1", uint64(SUBTRACTION))
	seed(444, "8 + 8 + 8", 12.0, 0, "llm_0.3", uint64(ADDITION|CHAINED_OPERATIONS))
	// A high scorer in the tail (difficulty 25.2) — buckets must extend to cover it.
	seed(555, "12x + 7 = 199", 25.2, 0, "llm_0.3", uint64(SINGLE_VARIABLE|MISSING_NUMBER))
	// Bucket 6, llm_0.3: two ADDITION rows (same bitmap) + one SUBTRACTION row.
	// The view shows ONE example per distinct bitmap, so this generator group
	// should render 2 examples, not 3.
	seed(601, "1 + 2", 6.1, 0, "llm_0.3", uint64(ADDITION))
	seed(602, "2 + 1", 6.2, 0, "llm_0.3", uint64(ADDITION))
	seed(603, "9 - 4", 6.3, 0, "llm_0.3", uint64(SUBTRACTION))

	// Student is forbidden by RequireAdmin.
	student := createTestUser(t, r, "auth0id|calib-student", "cs@test.com", "calibstudent")
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/admin/difficulty-calibration?test_auth0_id=%s", student.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d: %s", resp.Code, resp.Body.Bytes())
	}

	// Admin gets the JSON.
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/admin/difficulty-calibration?test_auth0_id=%s", admin.Auth0Id), nil)
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content type, got %q", ct)
	}

	var data CalibrationData
	if err := json.Unmarshal(resp.Body.Bytes(), &data); err != nil {
		t.Fatalf("unmarshal calibration JSON: %v", err)
	}
	// Buckets extend to cover the pool's max difficulty (25.2 → bucket center
	// 25), not a fixed 20.5 cap, so the heavy tail is visible.
	if len(data.Buckets) != 25 {
		t.Fatalf("expected 25 buckets (1..25, covering the max), got %d", len(data.Buckets))
	}

	findBucket := func(label string) *CalibrationBucket {
		for i := range data.Buckets {
			if data.Buckets[i].Label == label {
				return &data.Buckets[i]
			}
		}
		return nil
	}

	// The tail bucket exists and caught the high scorer.
	if b25 := findBucket("25"); b25 == nil {
		t.Errorf("bucket 25 (tail) not found")
	} else if b25.LiveCount != 1 {
		t.Errorf("bucket 25: expected 1 live, got %d", b25.LiveCount)
	}

	// Bucket 7 ([6.5, 7.5)): two live (3+4, 5*6), one disabled (9-1).
	b7 := findBucket("7")
	if b7 == nil {
		t.Fatalf("bucket 7 not found")
	}
	if b7.LiveCount != 2 || b7.DisabledCount != 1 {
		t.Errorf("bucket 7 counts: expected 2 live / 1 disabled, got %d / %d", b7.LiveCount, b7.DisabledCount)
	}
	// Two live generators (heuristic_1.0, llm_0.3), each its own group; the
	// disabled llm_0.1 problem is not sampled.
	if len(b7.Generators) != 2 {
		t.Fatalf("bucket 7: expected 2 generator groups, got %d", len(b7.Generators))
	}

	// The heuristic_1.0 group holds the multiplication problem, whose breakdown
	// carries the multiplication opWeight (2.2) — proves both the per-generator
	// grouping and that ComputeDifficultyBreakdown is wired through.
	var mulGroup *CalibrationGenGroup
	for i := range b7.Generators {
		if b7.Generators[i].Generator == "heuristic_1.0" {
			mulGroup = &b7.Generators[i]
		}
	}
	if mulGroup == nil {
		t.Fatalf("bucket 7: heuristic_1.0 group not found")
	}
	if mulGroup.LiveCount != 1 || len(mulGroup.Problems) != 1 {
		t.Fatalf("heuristic_1.0 group: expected 1 live / 1 sampled, got %d / %d", mulGroup.LiveCount, len(mulGroup.Problems))
	}
	p := mulGroup.Problems[0]
	if p.Expression != `5 \times 6` {
		t.Errorf("expected expression %q, got %q", `5 \times 6`, p.Expression)
	}
	if p.Breakdown.OpWeight != weightMul {
		t.Errorf("expected opWeight %.2f (multiplication), got %.2f", weightMul, p.Breakdown.OpWeight)
	}

	// Bucket 6, llm_0.3: 3 live rows but only 2 distinct bitmaps (ADDITION ×2,
	// SUBTRACTION ×1), so the group shows ONE example per bitmap = 2 examples.
	b6 := findBucket("6")
	if b6 == nil {
		t.Fatalf("bucket 6 not found")
	}
	var llmGroup *CalibrationGenGroup
	for i := range b6.Generators {
		if b6.Generators[i].Generator == "llm_0.3" {
			llmGroup = &b6.Generators[i]
		}
	}
	if llmGroup == nil {
		t.Fatalf("bucket 6: llm_0.3 group not found")
	}
	if llmGroup.LiveCount != 3 {
		t.Errorf("bucket 6 llm_0.3: expected live_count 3, got %d", llmGroup.LiveCount)
	}
	if len(llmGroup.Problems) != 2 {
		t.Errorf("bucket 6 llm_0.3: expected 2 examples (one per distinct bitmap), got %d", len(llmGroup.Problems))
	}
}
