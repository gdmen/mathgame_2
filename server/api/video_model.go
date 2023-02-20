// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateVideoTableSQL = `
    CREATE TABLE videos (
        id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
	title VARCHAR(128) NOT NULL,
	url VARCHAR(256) NOT NULL,
	start INT(5) NOT NULL,
	end INT(5) NOT NULL,
	enabled TINYINT NOT NULL,
	thumbnailurl VARCHAR(256) NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createVideoSQL = `INSERT INTO videos (title, url, start, end, enabled, thumbnailurl) VALUES (?, ?, ?, ?, ?, ?);`

	getVideoSQL = `SELECT * FROM videos WHERE id=?;`

	getVideoKeySQL = `SELECT id FROM videos WHERE title=? AND url=? AND start=? AND end=? AND enabled=? AND thumbnailurl=?;`

	listVideoSQL = `SELECT * FROM videos;`

	updateVideoSQL = `UPDATE videos SET title=?, url=?, start=?, end=?, enabled=?, thumbnailurl=? WHERE id=?;`

	deleteVideoSQL = `DELETE FROM videos WHERE id=?;`
)

type Video struct {
	Id           uint32 `json:"id" uri:"id"`
	Title        string `json:"title" uri:"title" form:"title"`
	URL          string `json:"url" uri:"url" form:"url"`
	Start        int32  `json:"start" uri:"start" form:"start"`
	End          int32  `json:"end" uri:"end" form:"end"`
	Enabled      bool   `json:"enabled" uri:"enabled" form:"enabled"`
	ThumbnailURL string `json:"thumbnailurl" uri:"thumbnailurl" form:"thumbnailurl"`
}

func (model Video) String() string {
	return fmt.Sprintf("Id: %v, Title: %v, URL: %v, Start: %v, End: %v, Enabled: %v, ThumbnailURL: %v", model.Id, model.Title, model.URL, model.Start, model.End, model.Enabled, model.ThumbnailURL)
}

type VideoManager struct {
	DB *sql.DB
}

func (m *VideoManager) Create(model *Video) (int, string, error) {
	status := http.StatusCreated
	result, err := m.DB.Exec(createVideoSQL, model.Title, model.URL, model.Start, model.End, model.Enabled, model.ThumbnailURL)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add video to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		err = m.DB.QueryRow(getVideoKeySQL, model.Title, model.URL, model.Start, model.End, model.Enabled, model.ThumbnailURL).Scan(&model.Id)
		if err != nil {
			msg := "Couldn't add video to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	last_id, err := result.LastInsertId()
	if err != nil {
		msg := "Couldn't add video to database"
		return http.StatusInternalServerError, msg, err
	}
	model.Id = uint32(last_id)

	return status, "", nil
}

func (m *VideoManager) Get(id uint32) (*Video, int, string, error) {
	model := &Video{}
	err := m.DB.QueryRow(getVideoSQL, id).Scan(&model.Id, &model.Title, &model.URL, &model.Start, &model.End, &model.Enabled, &model.ThumbnailURL)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a video with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get video from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *VideoManager) List() (*[]Video, int, string, error) {
	models := []Video{}
	rows, err := m.DB.Query(listVideoSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get videos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Video{}
		err = rows.Scan(&model.Id, &model.Title, &model.URL, &model.Start, &model.End, &model.Enabled, &model.ThumbnailURL)
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

func (m *VideoManager) CustomList(sql string) (*[]Video, int, string, error) {
	models := []Video{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get videos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Video{}
		err = rows.Scan(&model.Id, &model.Title, &model.URL, &model.Start, &model.End, &model.Enabled, &model.ThumbnailURL)
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

func (m *VideoManager) Update(model *Video) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateVideoSQL, model.Title, model.URL, model.Start, model.End, model.Enabled, model.ThumbnailURL, model.Id)
	if err != nil {
		msg := "Couldn't update video in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *VideoManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteVideoSQL, id)
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
