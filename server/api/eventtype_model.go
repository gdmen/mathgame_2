// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateEventtypeTableSQL = `
    CREATE TABLE eventtypes (
        id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
	name VARCHAR(32) NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createEventtypeSQL = `INSERT INTO eventtypes (name) VALUES (?);`

	getEventtypeSQL = `SELECT * FROM eventtypes WHERE id=?;`

	getEventtypeKeySQL = `SELECT id FROM eventtypes WHERE name=?;`

	listEventtypeSQL = `SELECT * FROM eventtypes;`

	updateEventtypeSQL = `UPDATE eventtypes SET name=? WHERE id=?;`

	deleteEventtypeSQL = `DELETE FROM eventtypes WHERE id=?;`
)

type Eventtype struct {
	Id   uint64 `json:"id" uri:"id"`
	Name string `json:"name" uri:"name" form:"name"`
}

func (model Eventtype) String() string {
	return fmt.Sprintf("Id: %v, Name: %v", model.Id, model.Name)
}

type EventtypeManager struct {
	DB *sql.DB
}

func (m *EventtypeManager) Create(model *Eventtype) (int, string, error) {
	status := http.StatusCreated
	result, err := m.DB.Exec(createEventtypeSQL, model.Name)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add eventtype to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		_ = m.DB.QueryRow(getEventtypeKeySQL, model.Name).Scan(&model.Id)

		return http.StatusOK, "", nil
	}

	last_id, err := result.LastInsertId()
	if err != nil {
		msg := "Couldn't add eventtype to database"
		return http.StatusInternalServerError, msg, err
	}
	model.Id = uint64(last_id)

	return status, "", nil
}

func (m *EventtypeManager) Get(id uint64) (*Eventtype, int, string, error) {
	model := &Eventtype{}
	err := m.DB.QueryRow(getEventtypeSQL, id).Scan(&model.Id, &model.Name)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a eventtype with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get eventtype from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *EventtypeManager) List() (*[]Eventtype, int, string, error) {
	models := []Eventtype{}
	rows, err := m.DB.Query(listEventtypeSQL)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get eventtypes from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Eventtype{}
		err = rows.Scan(&model.Id, &model.Name)
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

func (m *EventtypeManager) Update(model *Eventtype) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateEventtypeSQL, model.Name, model.Id)
	if err != nil {
		msg := "Couldn't update eventtype in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *EventtypeManager) Delete(id uint64) (int, string, error) {
	result, err := m.DB.Exec(deleteEventtypeSQL, id)
	if err != nil {
		msg := "Couldn't delete eventtype in database"
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
