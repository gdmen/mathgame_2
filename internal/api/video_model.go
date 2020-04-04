// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	// The id lines should be 'bigint' instead of 'integer'
	// but sqlite3 has a fucky primary key system.
	CreateVideoTableSQL = `
	CREATE TABLE videos (
		id INT AUTO_INCREMENT PRIMARY KEY,
		title VARCHAR(128) CHARACTER SET utf8 COLLATE utf8_bin NOT NULL,
		local_file_name VARCHAR(32) CHARACTER SET utf8 COLLATE utf8_bin NOT NULL UNIQUE,
		enabled TINYINT NOT NULL
	);`
	createSQL = `
	INSERT INTO videos(title, local_file_name, enabled) VALUES(?, ?, ?);`
	updateSQL = `
	UPDATE videos SET title=?, local_file_name=?, enabled=? WHERE id=?;`
	deleteSQL = `
	DELETE FROM videos WHERE id=?;`
	getSQL = `
	SELECT * FROM videos WHERE id=?;`
	getIdSQLdSQL = `
	SELECT id FROM videos WHERE local_file_name=?;`
	listSQL = `
	SELECT * FROM videos;`
)

// Video represents a reward video
type Video struct {
	Id            int64  `json:"id"`
	Title         string `json:"title" form:"title"`
	LocalFileName string `json:"local_file_name" form:"local_file_name"`
	Enabled       bool   `json:"enabled" form:"enabled"`
}

func (model Video) String() string {
	return fmt.Sprintf("Id: %d, Title: %s, LocalFileName: %s, Enabled: %t", model.Id, model.Title, model.LocalFileName, model.Enabled)
}

type VideoManager struct {
	DB *sql.DB
}

func (m *VideoManager) Create(model *Video) (int, string, error) {
	result, err := m.DB.Exec(createSQL, model.Title, model.LocalFileName, model.Enabled)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add video to database"
			return http.StatusInternalServerError, msg, err
		}
		// Get the id of the already existing model
		er := m.DB.QueryRow(getIdSQLdSQL, model.LocalFileName).Scan(&model.Id)
		if er != nil {
			panic("This should be impossible.")
		} else {
			return http.StatusOK, "", nil
		}
	}
	// Get the Id that was just auto-written to the database
	// Ignore errors (if the database doesn't support LastInsertId)
	id, _ := result.LastInsertId()
	model.Id = id
	return http.StatusCreated, "", nil
}

func (m *VideoManager) Update(model *Video) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateSQL, model.Title, model.LocalFileName, model.Enabled, model.Id)
	if err != nil {
		msg := "Couldn't update video in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *VideoManager) Delete(id int64) (int, string, error) {
	result, err := m.DB.Exec(deleteSQL, id)
	if err != nil {
		msg := "Couldn't delete video in database"
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

func (m *VideoManager) Get(id int64) (*Video, int, string, error) {
	model := &Video{}
	err := m.DB.QueryRow(getSQL, id).Scan(&model.Id, &model.Title, &model.LocalFileName, &model.Enabled)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a video with that Id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get video from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *VideoManager) List() (*[]Video, int, string, error) {
	models := []Video{}
	rows, err := m.DB.Query(listSQL)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get videos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Video{}
		err = rows.Scan(&model.Id, &model.Title, &model.LocalFileName, &model.Enabled)
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

func (m *VideoManager) Custom(sql string) (*[]Video, int, string, error) {
	models := []Video{}
	rows, err := m.DB.Query(sql)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get videos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Video{}
		err = rows.Scan(&model.Id, &model.Title, &model.LocalFileName, &model.Enabled)
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
