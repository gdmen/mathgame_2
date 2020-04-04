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
	CreateVideoSQL = `
	INSERT INTO videos(title, local_file_name, enabled) VALUES(?, ?, ?);`
	CreateMultipleVideosSQL_A = `
	INSERT INTO videos(title, local_file_name, enabled) VALUES`
	CreateMultipleVideosSQL_B = `(?, ?, ?)`
	UpdateVideoSQL            = `
	UPDATE videos SET title=?, local_file_name=?, enabled=? WHERE id=?;`
	DeleteVideoSQL = `
	DELETE FROM videos WHERE id=?;`
	GetVideoSQL = `
	SELECT * FROM videos WHERE id=?;`
	ListVideoSQL = `
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
	result, err := m.DB.Exec(CreateVideoSQL, model.Title, model.LocalFileName, model.Enabled)
	if err != nil {
		msg := "Couldn't add video to database"
		return http.StatusInternalServerError, msg, err
	}
	// Get the Id that was just auto-written to the database
	// Ignore errors (if the database doesn't support LastInsertId)
	id, _ := result.LastInsertId()
	model.Id = id
	return http.StatusCreated, "", nil
}

func (m *VideoManager) CreateMultiple(models []*Video) (int, string, error) {
	sql := CreateMultipleVideosSQL_A
	sql += strings.Repeat(CreateMultipleVideosSQL_B+",", len(models))
	sql = sql[:len(sql)-1] + ";"
	m_params := []interface{}{}
	for _, model := range models {
		m_params = append(m_params, model.Title, model.LocalFileName, model.Enabled)
	}
	// Try to add all the ms at once
	_, err := m.DB.Exec(sql, m_params...)
	if err != nil {
		msg := "Couldn't add videos to database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusCreated, "", nil
}

func (m *VideoManager) Update(model *Video) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(UpdateVideoSQL, model.Title, model.LocalFileName, model.Enabled, model.Id)
	if err != nil {
		msg := "Couldn't update video in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *VideoManager) Delete(id int64) (int, string, error) {
	result, err := m.DB.Exec(DeleteVideoSQL, id)
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
	err := m.DB.QueryRow(GetVideoSQL, id).Scan(&model.Id, &model.Title, &model.LocalFileName, &model.Enabled)
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
	rows, err := m.DB.Query(ListVideoSQL)
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
