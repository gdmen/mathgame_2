package main

import (
	"database/sql"
	"fmt"

	"garydmenezes.com/mathgame/server/api"
)

// Per-row action codes returned by recomputeProblemRow.
const (
	actionSkipped = "skipped" // Already at current DifficultyVersion - no work needed.
	actionStamped = "stamped" // Value matched current formula; only version forward-stamped.
	actionUpdated = "updated" // Value differed; both difficulty and version rewritten.
)

// recomputeProblemRow decides what to do for a single problems row and
// (unless dryRun) writes the corresponding UPDATE. Returns the action and
// the newly-computed difficulty (zero when skipped, since we didn't bother
// recomputing it).
func recomputeProblemRow(db *sql.DB, id uint32, expr, symbolic string, oldDiff float64, oldVer string, dryRun bool) (action string, newDiff float64, err error) {
	// Fast path: row was stamped under the current formula version.
	// ComputeProblemDifficulty is deterministic, so newDiff would equal
	// oldDiff. No recompute, no write.
	if oldVer == api.DifficultyVersion {
		return actionSkipped, 0, nil
	}
	newDiff = api.ComputeProblemDifficulty(expr, symbolic)
	if recomputeFuzzyEqual(newDiff, oldDiff) {
		// Value matches the current formula but the version stamp is
		// stale. Forward-stamp the version without changing the value.
		if !dryRun {
			if _, err = db.Exec(`UPDATE problems SET difficulty_version = ? WHERE id = ?`, api.DifficultyVersion, id); err != nil {
				return "", newDiff, fmt.Errorf("stamp version id=%d: %w", id, err)
			}
		}
		return actionStamped, newDiff, nil
	}
	if !dryRun {
		if _, err = db.Exec(`UPDATE problems SET difficulty = ?, difficulty_version = ? WHERE id = ?`, newDiff, api.DifficultyVersion, id); err != nil {
			return "", newDiff, fmt.Errorf("update id=%d: %w", id, err)
		}
	}
	return actionUpdated, newDiff, nil
}

func recomputeFuzzyEqual(a, b float64) bool {
	delta := a - b
	if delta < 0 {
		delta = -delta
	}
	return delta < 0.01
}
