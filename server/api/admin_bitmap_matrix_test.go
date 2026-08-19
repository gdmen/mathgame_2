package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/mathcore"
)

// The bitmap universe the matrix tests inject: a simple addition envelope, a
// LARGE (requires MEDIUM) envelope to exercise the stamped→settings
// normalization, and a chained sub/add envelope with a higher ceiling.
var matrixTestBitmaps = []mathcore.ProblemType{
	mathcore.ADDITION,
	mathcore.ADDITION | mathcore.MEDIUM_NUMBERS | mathcore.LARGE_NUMBERS,
	mathcore.ADDITION | mathcore.SUBTRACTION | mathcore.CHAINED_OPERATIONS,
}

func seedMatrixProblems(t *testing.T, api *Api) {
	t.Helper()
	seed := func(id int, expr string, diff float64, status string, bitmap uint64) {
		_, err := api.DB.Exec(
			"INSERT INTO problems (id, problem_type_bitmap, expression, answer, explanation, symbolic_expression, difficulty, status, generator, difficulty_version) VALUES (?,?,?,?,?,?,?,?,?,?)",
			id, bitmap, expr, "7", "", "", diff, status, "heuristic_2.0", "0.3")
		if err != nil {
			t.Fatalf("seed problem %d: %v", id, err)
		}
	}
	// A targetable ADDITION cell at bucket 3.
	seed(1, "3 + 4", 3.4, StatusActive, uint64(mathcore.ADDITION))
	// A non-active ADDITION row in the same cell — must NOT be counted.
	seed(2, "2 + 5", 3.4, StatusReported, uint64(mathcore.ADDITION))
	// An ADDITION row far above its ceiling (~4) at bucket 30 — over-ceiling.
	seed(3, "1 + 2", 30.0, StatusActive, uint64(mathcore.ADDITION))
	// A LARGE-without-MEDIUM stamp: the divergent bracket. Normalization ORs
	// MEDIUM in, folding it into the ADDITION|MEDIUM|LARGE envelope at bucket 6.
	seed(4, "100 + 200", 6.0, StatusActive, uint64(mathcore.ADDITION|mathcore.LARGE_NUMBERS))
}

// TestComputeBitmapMatrixReport verifies pool counting (non-active excluded), the
// difficulty axis, targetable vs over-ceiling cells, and the LARGE⇒MEDIUM
// normalization that folds divergent stamps into the settings envelope.
func TestComputeBitmapMatrixReport(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, _, cleanup := setupTestAPI(t, c)
	defer cleanup()
	seedMatrixProblems(t, api)

	data, err := api.computeBitmapMatrixReport(matrixTestBitmaps)
	if err != nil {
		t.Fatalf("computeBitmapMatrixReport: %v", err)
	}

	if data.AxisLo != int(mathcore.MinTargetDifficulty) {
		t.Errorf("AxisLo = %d, want %d", data.AxisLo, int(mathcore.MinTargetDifficulty))
	}
	// The over-ceiling ADDITION pool row at bucket 30 pushes the axis out.
	if data.AxisHi < 30 {
		t.Errorf("AxisHi = %d, want >= 30 (over-ceiling pool row at 30)", data.AxisHi)
	}
	if len(data.Rows) != len(matrixTestBitmaps) {
		t.Fatalf("got %d rows, want %d", len(data.Rows), len(matrixTestBitmaps))
	}

	// Rows are exactly the injected bitmaps, ascending.
	for i, r := range data.Rows {
		if r.Bitmap != uint64(matrixTestBitmaps[i]) {
			t.Errorf("row %d bitmap = %d, want %d", i, r.Bitmap, uint64(matrixTestBitmaps[i]))
		}
		if i > 0 && data.Rows[i-1].Bitmap >= r.Bitmap {
			t.Errorf("rows not ascending at %d: %d >= %d", i, data.Rows[i-1].Bitmap, r.Bitmap)
		}
	}

	// cellFor returns the cell for (bitmap, bucket) or nil if outside the row.
	cellFor := func(bitmap uint64, bucket int) *MatrixCell {
		for i := range data.Rows {
			if data.Rows[i].Bitmap != bitmap {
				continue
			}
			idx := bucket - data.AxisLo
			if idx < 0 || idx >= len(data.Rows[i].Cells) {
				return nil
			}
			return &data.Rows[i].Cells[idx]
		}
		return nil
	}

	// ADDITION bucket 3: one live row (non-active excluded), targetable → example.
	add3 := cellFor(uint64(mathcore.ADDITION), 3)
	if add3 == nil {
		t.Fatal("ADDITION bucket 3 cell missing")
	}
	if add3.Pool != 1 {
		t.Errorf("ADDITION bucket 3 pool = %d, want 1 (non-active row excluded)", add3.Pool)
	}
	if add3.Ex == "" || add3.Over {
		t.Errorf("ADDITION bucket 3 should be a targetable example, got %+v", *add3)
	}

	// ADDITION bucket 30: over the ceiling → greyed, pool shown, no example.
	add30 := cellFor(uint64(mathcore.ADDITION), 30)
	if add30 == nil {
		t.Fatal("ADDITION bucket 30 cell missing")
	}
	if !add30.Over || add30.Ex != "" || add30.Pool != 1 {
		t.Errorf("ADDITION bucket 30 want over-ceiling pool=1 no example, got %+v", *add30)
	}

	// LARGE|MEDIUM bucket 6: the LARGE-without-MEDIUM stamp folded in here.
	large6 := cellFor(uint64(mathcore.ADDITION|mathcore.MEDIUM_NUMBERS|mathcore.LARGE_NUMBERS), 6)
	if large6 == nil {
		t.Fatal("LARGE|MEDIUM bucket 6 cell missing")
	}
	if large6.Pool != 1 {
		t.Errorf("LARGE|MEDIUM bucket 6 pool = %d, want 1 (LARGE⇒MEDIUM normalization)", large6.Pool)
	}
}

// gunzipMatrix decodes a gzip response body into BitmapMatrixData.
func gunzipMatrix(t *testing.T, body []byte) BitmapMatrixData {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	var data BitmapMatrixData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	return data
}

// TestBitmapMatrixCacheEndpoints verifies the admin gate, that GET reports no
// cache before a recompute, and that POST rebuilds it into the gzipped cache.
func TestBitmapMatrixCacheEndpoints(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	seedMatrixProblems(t, api)
	api.bitmapMatrixBitmaps = matrixTestBitmaps // keep the recompute fast

	admin := createTestAdmin(t, api, r, "auth0id|matrix-admin", "matrix@test.com", "matrixadmin")

	get := func(auth string) *httptest.ResponseRecorder {
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/admin/bitmap-matrix?test_auth0_id=%s", auth), nil)
		r.ServeHTTP(resp, req)
		return resp
	}

	// Student is forbidden by RequireAdmin.
	student := createTestUser(t, r, "auth0id|matrix-student", "ms@test.com", "matrixstudent")
	if resp := get(student.Auth0Id); resp.Code != http.StatusForbidden {
		t.Errorf("expected 403 for student, got %d", resp.Code)
	}

	// Before any recompute: 200 with X-Has-Report false and no body.
	resp := get(admin.Auth0Id)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	if resp.Header().Get("X-Has-Report") != "false" {
		t.Errorf("expected X-Has-Report false before recompute, got %q", resp.Header().Get("X-Has-Report"))
	}

	// Trigger the background recompute.
	rr := httptest.NewRecorder()
	preq, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/admin/bitmap-matrix/recompute?test_auth0_id=%s", admin.Auth0Id), nil)
	r.ServeHTTP(rr, preq)
	if rr.Code != http.StatusOK {
		t.Fatalf("recompute: expected 200, got %d: %s", rr.Code, rr.Body.Bytes())
	}

	// Poll until the rebuild lands in the cache.
	var got *httptest.ResponseRecorder
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		got = get(admin.Auth0Id)
		if got.Header().Get("X-Has-Report") == "true" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got.Header().Get("X-Has-Report") != "true" {
		t.Fatal("report not populated after recompute")
	}
	if got.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding gzip, got %q", got.Header().Get("Content-Encoding"))
	}

	data := gunzipMatrix(t, got.Body.Bytes())
	if data.AxisLo != int(mathcore.MinTargetDifficulty) {
		t.Errorf("cached AxisLo = %d, want %d", data.AxisLo, int(mathcore.MinTargetDifficulty))
	}
	var addRow *MatrixRow
	for i := range data.Rows {
		if data.Rows[i].Bitmap == uint64(mathcore.ADDITION) {
			addRow = &data.Rows[i]
		}
	}
	if addRow == nil {
		t.Fatal("ADDITION row missing from cached report")
	}
	// The seeded over-ceiling row at bucket 30 (index 30-AxisLo) survives into
	// the cache as a greyed pool cell. (Assert here rather than on a low bucket:
	// creating the test users triggers background generation that inserts easy
	// ADDITION problems into the low buckets, so their counts are not fixed.)
	over := addRow.Cells[30-data.AxisLo]
	if !over.Over || over.Ex != "" || over.Pool != 1 {
		t.Errorf("cached ADDITION bucket 30 want over-ceiling pool=1 no example, got %+v", over)
	}

	// Let the goroutine fully exit before cleanup drops the DB.
	for i := 0; i < 100 && bitmapMatrixComputing.Load(); i++ {
		time.Sleep(50 * time.Millisecond)
	}
}

// TestBitmapMatrixCellEndpoint verifies the per-cell reroll: a valid targetable
// cell returns an example, and invalid/over-ceiling requests 400.
func TestBitmapMatrixCellEndpoint(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	admin := createTestAdmin(t, api, r, "auth0id|cell-admin", "cell@test.com", "celladmin")

	cell := func(bitmap, bucket string) *httptest.ResponseRecorder {
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf(
			"/api/v1/admin/bitmap-matrix/cell?bitmap=%s&bucket=%s&test_auth0_id=%s", bitmap, bucket, admin.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		return resp
	}

	// Valid targetable cell: ADDITION at bucket 3 → an example.
	resp := cell(fmt.Sprintf("%d", uint64(mathcore.ADDITION)), "3")
	if resp.Code != http.StatusOK {
		t.Fatalf("valid cell: expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}
	var mc MatrixCell
	if err := json.Unmarshal(resp.Body.Bytes(), &mc); err != nil {
		t.Fatalf("unmarshal cell: %v", err)
	}
	if mc.Ex == "" {
		t.Errorf("valid cell returned no example: %+v", mc)
	}

	// Invalid bitmap (no core op) → 400.
	if resp := cell(fmt.Sprintf("%d", uint64(mathcore.MEDIUM_NUMBERS)), "3"); resp.Code != http.StatusBadRequest {
		t.Errorf("invalid bitmap: expected 400, got %d", resp.Code)
	}
	// Over-ceiling bucket for ADDITION (ceiling ~4) → 400.
	if resp := cell(fmt.Sprintf("%d", uint64(mathcore.ADDITION)), "40"); resp.Code != http.StatusBadRequest {
		t.Errorf("over-ceiling bucket: expected 400, got %d", resp.Code)
	}
}
