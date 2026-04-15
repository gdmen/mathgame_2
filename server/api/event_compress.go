// Package api contains event compression for the events table.
package api

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

// summableEventTypes are event types whose value is duration (ms) and can be summed when consecutive.
var summableEventTypes = map[string]bool{
	WORKING_ON_PROBLEM: true,
	WATCHING_VIDEO:     true,
}

// CompressEvents returns updates (first row per run with value = sum) and events to delete (rest of each run).
// Input events must be sorted by (user_id, id). Single-event runs and non-summable types are left unchanged.
func CompressEvents(events []Event) (updates []Event, toDelete []Event) {
	if len(events) == 0 {
		return nil, nil
	}
	i := 0
	for i < len(events) {
		e := events[i]
		if !summableEventTypes[e.EventType] {
			i++
			continue
		}
		runStart := i
		sum, ok := parseEventDurationMs(e.Value)
		if !ok {
			i++
			continue
		}
		j := i + 1
		for j < len(events) && events[j].UserId == e.UserId && events[j].EventType == e.EventType {
			v, ok := parseEventDurationMs(events[j].Value)
			if !ok {
				break
			}
			sum += v
			j++
		}
		runEnd := j
		if runEnd-runStart <= 1 {
			i = runEnd
			continue
		}
		first := events[runStart]
		first.Value = strconv.FormatInt(sum, 10)
		updates = append(updates, first)
		for k := runStart + 1; k < runEnd; k++ {
			toDelete = append(toDelete, events[k])
		}
		i = runEnd
	}
	return updates, toDelete
}

// parseEventDurationMs parses an event row's value as a millisecond duration.
// Tolerates decimal strings (e.g. watching_video posts durations as floats like
// "505.9579275207507") by falling back to ParseFloat and truncating toward zero.
// Returns (0, false) only when the string cannot be parsed at all.
func parseEventDurationMs(s string) (int64, bool) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(f), true
}

const (
	selectEventsAfterIDSQL = `SELECT id, timestamp, user_id, event_type, value FROM events WHERE id > ? ORDER BY user_id, id`
	compressMetaGetSQL     = `SELECT last_event_id FROM compress_events_meta WHERE dummy = 1`
	compressMetaSetSQL     = `INSERT INTO compress_events_meta (dummy, last_event_id) VALUES (1, ?) ON DUPLICATE KEY UPDATE last_event_id = VALUES(last_event_id)`
	// maxChunkSize limits rows per batched UPDATE/DELETE to stay under MySQL's limit of 65,535 placeholders per prepared statement.
	// UPDATE uses 3 placeholders per row (id, value, id), so max = 65535/3 = 21845.
	maxChunkSize = 21845
)

func getLastCompressEventID(db *sql.DB) (uint64, error) {
	var id uint64
	err := db.QueryRow(compressMetaGetSQL).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func setLastCompressEventID(tx *sql.Tx, id uint64) error {
	_, err := tx.Exec(compressMetaSetSQL, id)
	return err
}

// planCompressFrom reads events with id > afterID, runs CompressEvents, and returns the event batch, planned updates, and deletes.
func planCompressFrom(db *sql.DB, afterID uint64) (events []Event, updates []Event, toDelete []Event, err error) {
	rows, err := db.Query(selectEventsAfterIDSQL, afterID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	var batch []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Id, &e.Timestamp, &e.UserId, &e.EventType, &e.Value); err != nil {
			return nil, nil, nil, err
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	updates, toDelete = CompressEvents(batch)
	return batch, updates, toDelete, nil
}

// PlanCompress reads events after the last checkpoint, returns planned updates and deletes (no write). Used for dry-run.
func PlanCompress(db *sql.DB) (updates []Event, toDelete []Event, err error) {
	afterID, err := getLastCompressEventID(db)
	if err != nil {
		return nil, nil, err
	}
	_, updates, toDelete, err = planCompressFrom(db, afterID)
	return updates, toDelete, err
}

// RunCompress reads events after the last checkpoint, compresses runs of summable types, applies changes, and advances the checkpoint in one transaction.
func RunCompress(db *sql.DB) (numUpdates, numDeletes int, err error) {
	afterID, err := getLastCompressEventID(db)
	if err != nil {
		return 0, 0, err
	}
	events, updates, toDelete, err := planCompressFrom(db, afterID)
	if err != nil {
		return 0, 0, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	numUpdates = len(updates)
	for off := 0; off < len(updates); off += maxChunkSize {
		end := off + maxChunkSize
		if end > len(updates) {
			end = len(updates)
		}
		chunk := updates[off:end]
		caseParts := make([]string, len(chunk))
		inPlaceholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)*3)
		for i, e := range chunk {
			caseParts[i] = "WHEN ? THEN ?"
			inPlaceholders[i] = "?"
			args = append(args, e.Id, e.Value)
		}
		for _, e := range chunk {
			args = append(args, e.Id)
		}
		_, err = tx.Exec(
			"UPDATE events SET value = CASE id "+strings.Join(caseParts, " ")+" END WHERE id IN ("+strings.Join(inPlaceholders, ",")+")",
			args...,
		)
		if err != nil {
			return 0, 0, err
		}
	}
	numDeletes = len(toDelete)
	for off := 0; off < len(toDelete); off += maxChunkSize {
		end := off + maxChunkSize
		if end > len(toDelete) {
			end = len(toDelete)
		}
		chunk := toDelete[off:end]
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for i, e := range chunk {
			placeholders[i] = "?"
			args[i] = e.Id
		}
		_, err = tx.Exec("DELETE FROM events WHERE id IN ("+strings.Join(placeholders, ",")+")", args...)
		if err != nil {
			return 0, 0, err
		}
	}

	var maxEventID uint64
	for _, e := range events {
		if uint64(e.Id) > maxEventID {
			maxEventID = uint64(e.Id)
		}
	}
	if maxEventID > 0 {
		if err = setLastCompressEventID(tx, maxEventID); err != nil {
			return 0, 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	glog.Infof("compress_events: updated %d rows, deleted %d rows", numUpdates, numDeletes)
	return numUpdates, numDeletes, nil
}
