// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	CreateEventTableSQL = `
    CREATE TABLE events (
        id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
	timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	user_id BIGINT UNSIGNED NOT NULL,
	event_type VARCHAR(32) NOT NULL,
	value TEXT NOT NULL
    ) DEFAULT CHARSET=utf8mb4 ;`

	createEventSQL = `INSERT INTO events (user_id, event_type, value) VALUES (?, ?, ?);`

	getEventSQL = `SELECT * FROM events WHERE id=? AND user_id=?;`

	getEventKeySQL = `SELECT id FROM events WHERE user_id=? AND event_type=? AND value=?;`

	listEventSQL = `SELECT * FROM events WHERE user_id=?;`

	updateEventSQL = `UPDATE events SET timestamp=?, user_id=?, event_type=?, value=? WHERE id=? AND user_id=?;`

	deleteEventSQL = `DELETE FROM events WHERE id=? AND user_id=?;`
)

type Event struct {
	Id        uint32    `json:"id" uri:"id"`
	Timestamp time.Time `json:"timestamp" uri:"timestamp" form:"timestamp"`
	UserId    uint32    `json:"user_id" uri:"user_id" form:"user_id"`
	EventType string    `json:"event_type" uri:"event_type" form:"event_type"`
	Value     string    `json:"value" uri:"value" form:"value"`
}

func (model Event) String() string {
	return fmt.Sprintf("Id: %v, Timestamp: %v, UserId: %v, EventType: %v, Value: %v", model.Id, model.Timestamp, model.UserId, model.EventType, model.Value)
}

type EventManager struct {
	DB *sql.DB
}

func (m *EventManager) Create(model *Event) (int, string, error) {
	status := http.StatusCreated
	result, err := m.DB.Exec(createEventSQL, model.UserId, model.EventType, model.Value)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add event to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		err = m.DB.QueryRow(getEventKeySQL, model.UserId, model.EventType, model.Value).Scan(&model.Id)
		if err != nil {
			msg := "Couldn't add event to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	last_id, err := result.LastInsertId()
	if err != nil {
		msg := "Couldn't add event to database"
		return http.StatusInternalServerError, msg, err
	}
	model.Id = uint32(last_id)

	return status, "", nil
}

func (m *EventManager) Get(id uint32, user_id uint32) (*Event, int, string, error) {
	model := &Event{}
	err := m.DB.QueryRow(getEventSQL, id, user_id).Scan(&model.Id, &model.Timestamp, &model.UserId, &model.EventType, &model.Value)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a event with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get event from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *EventManager) List(user_id uint32) (*[]Event, int, string, error) {
	models := []Event{}
	rows, err := m.DB.Query(listEventSQL, user_id)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get events from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Event{}
		err = rows.Scan(&model.Id, &model.Timestamp, &model.UserId, &model.EventType, &model.Value)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		models = append(models, model)
	}
	err = rows.Err()
	if err != nil {
		msg := "Error scanning rows from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return &models, http.StatusOK, "", nil
}

func (m *EventManager) CustomList(sql string) (*[]Event, int, string, error) {
	models := []Event{}
	sql = "SELECT * FROM events WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get events from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Event{}
		err = rows.Scan(&model.Id, &model.Timestamp, &model.UserId, &model.EventType, &model.Value)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		models = append(models, model)
	}
	err = rows.Err()
	if err != nil {
		msg := "Error scanning rows from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return &models, http.StatusOK, "", nil
}

func (m *EventManager) CustomIdList(sql string) (*[]uint32, int, string, error) {
	ids := []uint32{}
	sql = "SELECT id FROM events WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get events from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		var id uint32
		err = rows.Scan(&id)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	if err != nil {
		msg := "Error scanning rows from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return &ids, http.StatusOK, "", nil
}

func (m *EventManager) CustomSql(sql string) (int, string, error) {
	_, err := m.DB.Query(sql)
	if err != nil {
		msg := "Couldn't run sql for Event in database"
		return http.StatusBadRequest, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *EventManager) Update(model *Event, user_id uint32) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id, user_id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateEventSQL, model.Timestamp, model.UserId, model.EventType, model.Value, model.Id, user_id)
	if err != nil {
		msg := "Couldn't update event in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *EventManager) Delete(id uint32, user_id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteEventSQL, id, user_id)
	if err != nil {
		msg := "Couldn't delete event in database"
		return http.StatusInternalServerError, msg, err
	}
	// Check for 404s
	// Ignore errors (if the database doesn't support RowsAffected)
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return http.StatusNotFound, "", nil
	}
	return http.StatusNoContent, "", nil
}
