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
	user_id BIGINT UNSIGNED NOT NULL,
	title VARCHAR(128) NOT NULL,
	url VARCHAR(256) NOT NULL,
	thumbnailurl VARCHAR(256) NOT NULL,
	disabled TINYINT NOT NULL DEFAULT 0,
	deleted TINYINT NOT NULL DEFAULT 0
    ) DEFAULT CHARSET=utf8mb4 ;`

	createVideoSQL = `INSERT INTO videos (user_id, title, url, thumbnailurl) VALUES (?, ?, ?, ?);`

	getVideoSQL = `SELECT * FROM videos WHERE id=? AND user_id=?;`

	getVideoKeySQL = `SELECT id FROM videos WHERE user_id=? AND title=? AND url=? AND thumbnailurl=?;`

	listVideoSQL = `SELECT * FROM videos WHERE user_id=?;`

	updateVideoSQL = `UPDATE videos SET user_id=?, title=?, url=?, thumbnailurl=?, disabled=?, deleted=? WHERE id=? AND user_id=?;`

	deleteVideoSQL = `DELETE FROM videos WHERE id=? AND user_id=?;`
)

type Video struct {
	Id           uint32 `json:"id" uri:"id"`
	UserId       uint32 `json:"user_id" uri:"user_id" form:"user_id"`
	Title        string `json:"title" uri:"title" form:"title"`
	URL          string `json:"url" uri:"url" form:"url"`
	ThumbnailURL string `json:"thumbnailurl" uri:"thumbnailurl" form:"thumbnailurl"`
	Disabled     bool   `json:"disabled" uri:"disabled" form:"disabled"`
	Deleted      bool   `json:"deleted" uri:"deleted" form:"deleted"`
}

func (model Video) String() string {
	return fmt.Sprintf("Id: %v, UserId: %v, Title: %v, URL: %v, ThumbnailURL: %v, Disabled: %v, Deleted: %v", model.Id, model.UserId, model.Title, model.URL, model.ThumbnailURL, model.Disabled, model.Deleted)
}

type VideoManager struct {
	DB *sql.DB
}

func (m *VideoManager) Create(model *Video) (int, string, error) {
	status := http.StatusCreated
	result, err := m.DB.Exec(createVideoSQL, model.UserId, model.Title, model.URL, model.ThumbnailURL)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add video to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		err = m.DB.QueryRow(getVideoKeySQL, model.UserId, model.Title, model.URL, model.ThumbnailURL).Scan(&model.Id)
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

func (m *VideoManager) Get(id uint32, user_id uint32) (*Video, int, string, error) {
	model := &Video{}
	err := m.DB.QueryRow(getVideoSQL, id, user_id).Scan(&model.Id, &model.UserId, &model.Title, &model.URL, &model.ThumbnailURL, &model.Disabled, &model.Deleted)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a video with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get video from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *VideoManager) List(user_id uint32) (*[]Video, int, string, error) {
	models := []Video{}
	rows, err := m.DB.Query(listVideoSQL, user_id)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get videos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Video{}
		err = rows.Scan(&model.Id, &model.UserId, &model.Title, &model.URL, &model.ThumbnailURL, &model.Disabled, &model.Deleted)
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
		err = rows.Scan(&model.Id, &model.UserId, &model.Title, &model.URL, &model.ThumbnailURL, &model.Disabled, &model.Deleted)
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

func (m *VideoManager) CustomSql(sql string) (int, string, error) {
	_, err := m.DB.Query(sql)
	if err != nil {
		msg := "Couldn't run sql for Video in database"
		return http.StatusBadRequest, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *VideoManager) Update(model *Video, user_id uint32) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id, user_id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateVideoSQL, model.UserId, model.Title, model.URL, model.ThumbnailURL, model.Disabled, model.Deleted, model.Id, user_id)
	if err != nil {
		msg := "Couldn't update video in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *VideoManager) Delete(id uint32, user_id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteVideoSQL, id, user_id)
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
