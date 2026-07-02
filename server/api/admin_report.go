package api

import (
	"database/sql"
	"sync/atomic"

	"github.com/golang/glog"
)

// startBackgroundReport runs compute() in a background goroutine under the
// computing guard and upserts its bytes into a single-row cache table via
// upsertSQL (which must take exactly one '?' placeholder for the blob). It
// returns immediately; a second call while a rebuild is already in flight is a
// no-op. Shared by the admin difficulty-calibration and bitmap-matrix report
// caches, which differ only in how they serialize their report.
func startBackgroundReport(db *sql.DB, computing *atomic.Bool, logPrefix, label, upsertSQL string, compute func() ([]byte, error)) {
	if !computing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer computing.Store(false)
		defer func() {
			if r := recover(); r != nil {
				glog.Errorf("%s %s recompute panicked: %v", logPrefix, label, r)
			}
		}()
		blob, err := compute()
		if err != nil {
			glog.Errorf("%s %s recompute: %v", logPrefix, label, err)
			return
		}
		if _, err := db.Exec(upsertSQL, blob); err != nil {
			glog.Errorf("%s %s cache write: %v", logPrefix, label, err)
		}
	}()
}
