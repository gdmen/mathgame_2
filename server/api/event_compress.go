// Package api contains event compression for the events table.
package api

import (
	"database/sql"
	"strconv"

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
		sum, ok := parseInt64(e.Value)
		if !ok {
			i++
			continue
		}
		j := i + 1
		for j < len(events) && events[j].UserId == e.UserId && events[j].EventType == e.EventType {
			v, ok := parseInt64(events[j].Value)
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

func parseInt64(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

const (
	selectEventsOrderedSQL = `SELECT id, timestamp, user_id, event_type, value FROM events ORDER BY user_id, id`
	compressUpdateEventSQL = `UPDATE events SET timestamp=?, user_id=?, event_type=?, value=? WHERE id=? AND user_id=?`
	compressDeleteEventSQL = `DELETE FROM events WHERE id=? AND user_id=?`
)

// PlanCompress reads all events and returns planned updates and deletes (no write). Used for dry-run.
func PlanCompress(db *sql.DB) (updates []Event, toDelete []Event, err error) {
	rows, err := db.Query(selectEventsOrderedSQL)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Id, &e.Timestamp, &e.UserId, &e.EventType, &e.Value); err != nil {
			return nil, nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	updates, toDelete = CompressEvents(events)
	return updates, toDelete, nil
}

// RunCompress reads all events, compresses runs of summable types (update first row, delete rest), and applies changes in one transaction.
func RunCompress(db *sql.DB) (numUpdates, numDeletes int, err error) {
	updates, toDelete, err := PlanCompress(db)
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

	for _, e := range updates {
		_, err = tx.Exec(compressUpdateEventSQL, e.Timestamp, e.UserId, e.EventType, e.Value, e.Id, e.UserId)
		if err != nil {
			return 0, 0, err
		}
		numUpdates++
	}
	for _, e := range toDelete {
		_, err = tx.Exec(compressDeleteEventSQL, e.Id, e.UserId)
		if err != nil {
			return 0, 0, err
		}
		numDeletes++
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	glog.Infof("compress_events: updated %d rows, deleted %d rows", numUpdates, numDeletes)
	return numUpdates, numDeletes, nil
}
