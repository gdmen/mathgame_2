package api

import (
	"testing"
	"time"

	"garydmenezes.com/mathgame/server/common"
)

func ts(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func TestCompressEvents_Empty(t *testing.T) {
	updates, toDelete := CompressEvents(nil)
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("empty input: want updates=0, toDelete=0; got %d, %d", len(updates), len(toDelete))
	}
	updates, toDelete = CompressEvents([]Event{})
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("empty slice: want updates=0, toDelete=0; got %d, %d", len(updates), len(toDelete))
	}
}

func TestCompressEvents_SingleEvent(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 100, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(1)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("single event: want no update/delete; got updates=%d, toDelete=%d", len(updates), len(toDelete))
	}
}

func TestCompressEvents_ThreeConsecutiveWatchingVideo(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 100, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(1)},
		{Id: 2, UserId: 100, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(2)},
		{Id: 3, UserId: 100, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(3)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 1 {
		t.Fatalf("updates: want 1, got %d", len(updates))
	}
	if updates[0].Id != 1 || updates[0].Value != "15000" || updates[0].UserId != 100 || updates[0].EventType != WATCHING_VIDEO {
		t.Errorf("update: want id=1 value=15000; got %+v", updates[0])
	}
	if len(toDelete) != 2 {
		t.Fatalf("toDelete: want 2, got %d", len(toDelete))
	}
	if toDelete[0].Id != 2 || toDelete[1].Id != 3 {
		t.Errorf("toDelete ids: want [2,3], got [%d,%d]", toDelete[0].Id, toDelete[1].Id)
	}
}

func TestCompressEvents_TwoConsecutiveWorkingOnProblem(t *testing.T) {
	events := []Event{
		{Id: 10, UserId: 1, EventType: WORKING_ON_PROBLEM, Value: "1000", Timestamp: ts(10)},
		{Id: 11, UserId: 1, EventType: WORKING_ON_PROBLEM, Value: "2000", Timestamp: ts(11)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 1 || updates[0].Value != "3000" || updates[0].Id != 10 {
		t.Errorf("updates: want one with value 3000 id 10; got %+v", updates)
	}
	if len(toDelete) != 1 || toDelete[0].Id != 11 {
		t.Errorf("toDelete: want [id=11]; got %+v", toDelete)
	}
}

func TestCompressEvents_InterleavedTypes(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 1, EventType: WORKING_ON_PROBLEM, Value: "1000", Timestamp: ts(1)},
		{Id: 2, UserId: 1, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(2)},
		{Id: 3, UserId: 1, EventType: WORKING_ON_PROBLEM, Value: "2000", Timestamp: ts(3)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("interleaved: want no merge; got updates=%d, toDelete=%d", len(updates), len(toDelete))
	}
}

func TestCompressEvents_TwoRunsSameType(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 1, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(1)},
		{Id: 2, UserId: 1, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(2)},
		{Id: 3, UserId: 1, EventType: SOLVED_PROBLEM, Value: "42", Timestamp: ts(3)},
		{Id: 4, UserId: 1, EventType: WATCHING_VIDEO, Value: "1000", Timestamp: ts(4)},
		{Id: 5, UserId: 1, EventType: WATCHING_VIDEO, Value: "2000", Timestamp: ts(5)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 2 {
		t.Fatalf("updates: want 2 runs; got %d", len(updates))
	}
	if updates[0].Id != 1 || updates[0].Value != "10000" {
		t.Errorf("first run: want id=1 value=10000; got id=%d value=%s", updates[0].Id, updates[0].Value)
	}
	if updates[1].Id != 4 || updates[1].Value != "3000" {
		t.Errorf("second run: want id=4 value=3000; got id=%d value=%s", updates[1].Id, updates[1].Value)
	}
	if len(toDelete) != 2 {
		t.Fatalf("toDelete: want 2; got %d", len(toDelete))
	}
	if toDelete[0].Id != 2 || toDelete[1].Id != 5 {
		t.Errorf("toDelete ids: want [2,5]; got [%d,%d]", toDelete[0].Id, toDelete[1].Id)
	}
}

func TestCompressEvents_NonNumericValueSkipsRun(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 1, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(1)},
		{Id: 2, UserId: 1, EventType: WATCHING_VIDEO, Value: "bad", Timestamp: ts(2)},
		{Id: 3, UserId: 1, EventType: WATCHING_VIDEO, Value: "5000", Timestamp: ts(3)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("non-numeric in run: want no merge; got updates=%d, toDelete=%d", len(updates), len(toDelete))
	}
}

func TestCompressEvents_NonSummableLeftAlone(t *testing.T) {
	events := []Event{
		{Id: 1, UserId: 1, EventType: SOLVED_PROBLEM, Value: "1", Timestamp: ts(1)},
		{Id: 2, UserId: 1, EventType: SOLVED_PROBLEM, Value: "2", Timestamp: ts(2)},
	}
	updates, toDelete := CompressEvents(events)
	if len(updates) != 0 || len(toDelete) != 0 {
		t.Errorf("non-summable: want no merge; got updates=%d, toDelete=%d", len(updates), len(toDelete))
	}
}

func TestRunCompress_Integration_ProgressUnchanged(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|compress-progress", "compress@test.com", "compressuser")

	// Insert 3x watching_video 5000 ms each (total 15000 ms = 0.25 min, but progress rounds; we'll assert row count and same total)
	_, err = api.DB.Exec(
		"INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		user.Id, WATCHING_VIDEO, "5000",
		user.Id, WATCHING_VIDEO, "5000",
		user.Id, WATCHING_VIDEO, "5000",
	)
	if err != nil {
		t.Fatalf("insert events: %v", err)
	}

	// Get progress before compress (total_video_minutes: 15000/60000 = 0)
	var count int
	err = api.DB.QueryRow("SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?", user.Id, WATCHING_VIDEO).Scan(&count)
	if err != nil || count != 3 {
		t.Fatalf("before: want 3 rows; got count=%d err=%v", count, err)
	}

	numUpdates, numDeletes, err := RunCompress(api.DB)
	if err != nil {
		t.Fatalf("RunCompress: %v", err)
	}
	if numUpdates != 1 || numDeletes != 2 {
		t.Errorf("RunCompress: want 1 update, 2 deletes; got %d, %d", numUpdates, numDeletes)
	}

	err = api.DB.QueryRow("SELECT COUNT(*) FROM events WHERE user_id = ? AND event_type = ?", user.Id, WATCHING_VIDEO).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("after: want 1 row; got count=%d err=%v", count, err)
	}

	var value string
	err = api.DB.QueryRow("SELECT value FROM events WHERE user_id = ? AND event_type = ?", user.Id, WATCHING_VIDEO).Scan(&value)
	if err != nil || value != "15000" {
		t.Errorf("merged value: want 15000; got %q err=%v", value, err)
	}
	// Sum preserved: 5000+5000+5000 = 15000 (any progress/totals query would see same total)
}
