package api

import (
	"encoding/json"
	"math"
	"testing"

	"garydmenezes.com/mathgame/server/mathcore"
)

// TestDifficultyBandFixturesSync pins the shared parity fixtures
// (web/src/difficulty_band_fixtures.json) to the Go difficulty band formulas.
// The fixtures are the bridge that keeps the hand-maintained JS mirror
// (bitmap_validation.js) in lockstep with the Go source of truth: this test
// fails when the Go formula moves away from the checked-in fixtures
// (regenerate with `make gen-difficulty-fixtures`), and the JS test consumes
// the same file, failing when the mirror lags the regenerated values.
func TestDifficultyBandFixturesSync(t *testing.T) {
	const path = "../../web/src/difficulty_band_fixtures.json"
	data := readFileForTest(t, path)

	var fixtures []struct {
		Name string  `json:"name"`
		Bits uint64  `json:"bits"`
		Max  float64 `json:"max"`
		Min  float64 `json:"min"`
		Lo   float64 `json:"lo"`
		Hi   float64 `json:"hi"`
	}
	if err := json.Unmarshal([]byte(data), &fixtures); err != nil {
		t.Fatalf("%s unparseable: %v", path, err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("%s is empty - regenerate with `make gen-difficulty-fixtures`", path)
	}

	const tol = 1e-9
	for _, f := range fixtures {
		if got := mathcore.MaxDiffForBitmap(f.Bits); math.Abs(got-f.Max) > tol {
			t.Errorf("%s: MaxDiffForBitmap(%d) = %v, fixture says %v - regenerate with `make gen-difficulty-fixtures` (and update the JS mirror)",
				f.Name, f.Bits, got, f.Max)
		}
		if got := mathcore.MinDiffForBitmap(f.Bits); math.Abs(got-f.Min) > tol {
			t.Errorf("%s: MinDiffForBitmap(%d) = %v, fixture says %v - regenerate with `make gen-difficulty-fixtures` (and update the JS mirror)",
				f.Name, f.Bits, got, f.Min)
		}
		lo, hi := mathcore.TargetDifficultyRange(f.Bits)
		if math.Abs(lo-f.Lo) > tol || math.Abs(hi-f.Hi) > tol {
			t.Errorf("%s: TargetDifficultyRange(%d) = [%v, %v], fixture says [%v, %v] - regenerate with `make gen-difficulty-fixtures` (and update the JS mirror)",
				f.Name, f.Bits, lo, hi, f.Lo, f.Hi)
		}
	}
}
