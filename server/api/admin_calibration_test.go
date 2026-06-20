package api // import "garydmenezes.com/mathgame/server/api"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
)

func seedCalibrationProblems(t *testing.T, api *Api) {
	t.Helper()
	seed := func(id int, expr string, diff float64, disabled int, gen string, bitmap uint64) {
		_, err := api.DB.Exec(
			"INSERT INTO problems (id, problem_type_bitmap, expression, answer, explanation, symbolic_expression, difficulty, disabled, generator, difficulty_version) VALUES (?,?,?,?,?,?,?,?,?,?)",
			id, bitmap, expr, "7", "", "", diff, disabled, gen, "0.2")
		if err != nil {
			t.Fatalf("seed problem %d: %v", id, err)
		}
	}
	// Two live + one disabled in bucket 7; one live in bucket 12; a tail scorer
	// in bucket 25; and bucket 6 with two same-bitmap rows + one other.
	seed(111, "3 + 4", 7.2, 0, "llm_0.3", uint64(ADDITION))
	seed(222, `5 \times 6`, 7.4, 0, "heuristic_1.0", uint64(MULTIPLICATION))
	seed(333, "9 - 1", 7.1, 1, "llm_0.1", uint64(SUBTRACTION))
	seed(444, "8 + 8 + 8", 12.0, 0, "llm_0.3", uint64(ADDITION|CHAINED_OPERATIONS))
	seed(555, "12x + 7 = 199", 25.2, 0, "llm_0.3", uint64(SINGLE_VARIABLE|MISSING_NUMBER))
	seed(601, "1 + 2", 6.1, 0, "llm_0.3", uint64(ADDITION))
	seed(602, "2 + 1", 6.2, 0, "llm_0.3", uint64(ADDITION))
	seed(603, "9 - 4", 6.3, 0, "llm_0.3", uint64(SUBTRACTION))
}

// TestComputeCalibrationReport verifies the report computation: buckets span the
// max difficulty, disabled rows count separately, and each generator group
// shows one example per distinct problem-type bitmap with its factor breakdown.
func TestComputeCalibrationReport(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()
	seedCalibrationProblems(t, api)

	data, err := api.computeCalibrationReport()
	if err != nil {
		t.Fatalf("computeCalibrationReport: %v", err)
	}
	// Buckets extend to cover the pool's max difficulty (25.2 → bucket 25).
	if len(data.Buckets) != 25 {
		t.Fatalf("expected 25 buckets (1..25), got %d", len(data.Buckets))
	}

	findBucket := func(label string) *CalibrationBucket {
		for i := range data.Buckets {
			if data.Buckets[i].Label == label {
				return &data.Buckets[i]
			}
		}
		return nil
	}

	if b25 := findBucket("25"); b25 == nil {
		t.Errorf("bucket 25 (tail) not found")
	} else if b25.LiveCount != 1 {
		t.Errorf("bucket 25: expected 1 live, got %d", b25.LiveCount)
	}

	// Bucket 7: two live (3+4, 5*6), one disabled (9-1); two generator groups.
	b7 := findBucket("7")
	if b7 == nil {
		t.Fatalf("bucket 7 not found")
	}
	if b7.LiveCount != 2 || b7.DisabledCount != 1 {
		t.Errorf("bucket 7 counts: expected 2 live / 1 disabled, got %d / %d", b7.LiveCount, b7.DisabledCount)
	}
	if len(b7.Generators) != 2 {
		t.Fatalf("bucket 7: expected 2 generator groups, got %d", len(b7.Generators))
	}

	// The heuristic_1.0 group's example carries the multiplication opWeight —
	// proves the per-generator grouping and that the breakdown is wired through.
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
		t.Fatalf("heuristic_1.0 group: expected 1 live / 1 example, got %d / %d", mulGroup.LiveCount, len(mulGroup.Problems))
	}
	if p := mulGroup.Problems[0]; p.Expression != `5 \times 6` || p.Breakdown.OpWeight != weightMul {
		t.Errorf("heuristic_1.0 example: expr %q opWeight %.2f, want %q / %.2f", p.Expression, p.Breakdown.OpWeight, `5 \times 6`, weightMul)
	}

	// Bucket 6, llm_0.3: 3 live but 2 distinct bitmaps → 2 examples.
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

// TestCalibrationCacheEndpoints verifies the admin gate, that GET reads the
// cache (empty until computed), and that POST recompute rebuilds it.
func TestCalibrationCacheEndpoints(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	seedCalibrationProblems(t, api)

	admin := createTestUser(t, r, "auth0id|calib-admin", "calib@test.com", "calibadmin")
	if _, err := api.DB.Exec("UPDATE users SET role=? WHERE auth0_id=?", RoleAdmin, admin.Auth0Id); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	getReport := func(auth string) (*httptest.ResponseRecorder, CalibrationReportResponse) {
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/admin/difficulty-calibration?test_auth0_id=%s", auth), nil)
		r.ServeHTTP(resp, req)
		var body CalibrationReportResponse
		_ = json.Unmarshal(resp.Body.Bytes(), &body)
		return resp, body
	}

	// Student is forbidden by RequireAdmin.
	student := createTestUser(t, r, "auth0id|calib-student", "cs@test.com", "calibstudent")
	if resp, _ := getReport(student.Auth0Id); resp.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", resp.Code)
	}

	// Before any recompute: 200 with a nil report.
	resp, body := getReport(admin.Auth0Id)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	if body.Report != nil {
		t.Errorf("expected nil report before recompute")
	}

	// Trigger the background recompute.
	rr := httptest.NewRecorder()
	preq, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/admin/difficulty-calibration/recompute?test_auth0_id=%s", admin.Auth0Id), nil)
	r.ServeHTTP(rr, preq)
	if rr.Code != http.StatusOK {
		t.Fatalf("recompute: expected 200, got %d: %s", rr.Code, rr.Body.Bytes())
	}

	// Poll until the rebuild lands in the cache.
	var got CalibrationReportResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, got = getReport(admin.Auth0Id); got.Report != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got.Report == nil {
		t.Fatal("report not populated after recompute")
	}
	var b7 *CalibrationBucket
	for i := range got.Report.Buckets {
		if got.Report.Buckets[i].Label == "7" {
			b7 = &got.Report.Buckets[i]
		}
	}
	if b7 == nil || b7.LiveCount != 2 || b7.DisabledCount != 1 {
		t.Errorf("bucket 7 from cached report wrong: %+v", b7)
	}

	// Let the goroutine fully exit before cleanup drops the DB.
	for i := 0; i < 50 && calibrationComputing.Load(); i++ {
		time.Sleep(50 * time.Millisecond)
	}
}
