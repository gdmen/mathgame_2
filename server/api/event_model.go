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
    ) DEFAULT CHARSET=utf8 ;`

	createEventSQL = `INSERT INTO events (timestamp, user_id, event_type, value) VALUES (?, ?, ?, ?);`

	getEventSQL = `SELECT * FROM events WHERE id=?;`

	getEventKeySQL = `SELECT id FROM events WHERE timestamp=?, user_id=?, event_type=?, value=?;`

	listEventSQL = `SELECT * FROM events;`

	updateEventSQL = `UPDATE events SET timestamp=?, user_id=?, event_type=?, value=? WHERE id=?;`

	deleteEventSQL = `DELETE FROM events WHERE id=?;`
)

type Event struct {
	Id        uint64    `json:"id" uri:"id"`
	Timestamp time.Time `json:"timestamp" uri:"timestamp" form:"timestamp"`
	UserId    uint64    `json:"user_id" uri:"user_id" form:"user_id"`
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
	result, err := m.DB.Exec(createEventSQL, model.Timestamp, model.UserId, model.EventType, model.Value)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add event to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		_ = m.DB.QueryRow(getEventKeySQL, model.Timestamp, model.UserId, model.EventType, model.Value).Scan(&model.Id)

		return http.StatusOK, "", nil
	}

	last_id, err := result.LastInsertId()
	if err != nil {
		msg := "Couldn't add event to database"
		return http.StatusInternalServerError, msg, err
	}
	model.Id = uint64(last_id)

	return status, "", nil
}

func (m *EventManager) Get(id uint64) (*Event, int, string, error) {
	model := &Event{}
	err := m.DB.QueryRow(getEventSQL, id).Scan(&model.Id, &model.Timestamp, &model.UserId, &model.EventType, &model.Value)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a event with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get event from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *EventManager) List() (*[]Event, int, string, error) {
	models := []Event{}
	rows, err := m.DB.Query(listEventSQL)
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

func (m *EventManager) Update(model *Event) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateEventSQL, model.Timestamp, model.UserId, model.EventType, model.Value, model.Id)
	if err != nil {
		msg := "Couldn't update event in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *EventManager) Delete(id uint64) (int, string, error) {
	result, err := m.DB.Exec(deleteEventSQL, id)
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
