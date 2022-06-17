// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateUserhasvideoTableSQL = `
    CREATE TABLE userHasVideos (
        id BIGINT UNSIGNED PRIMARY KEY UNIQUE,
	user_id BIGINT UNSIGNED NOT NULL,
	video_id BIGINT UNSIGNED NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createUserhasvideoSQL = `INSERT INTO userHasVideos (id, user_id, video_id) VALUES (?, ?, ?);`

	getUserhasvideoSQL = `SELECT * FROM userHasVideos WHERE id=?;`

	getUserhasvideoKeySQL = `SELECT  FROM userHasVideos WHERE id=? AND user_id=? AND video_id=?;`

	listUserhasvideoSQL = `SELECT * FROM userHasVideos;`

	updateUserhasvideoSQL = `UPDATE userHasVideos SET user_id=?, video_id=? WHERE id=?;`

	deleteUserhasvideoSQL = `DELETE FROM userHasVideos WHERE id=?;`
)

type Userhasvideo struct {
	Id      uint32 `json:"id" uri:"id"`
	UserId  uint32 `json:"user_id" uri:"user_id" form:"user_id"`
	VideoId uint32 `json:"video_id" uri:"video_id" form:"video_id"`
}

func (model Userhasvideo) String() string {
	return fmt.Sprintf("Id: %v, UserId: %v, VideoId: %v", model.Id, model.UserId, model.VideoId)
}

type UserhasvideoManager struct {
	DB *sql.DB
}

func (m *UserhasvideoManager) Create(model *Userhasvideo) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createUserhasvideoSQL, model.Id, model.UserId, model.VideoId)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add userHasVideo to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *UserhasvideoManager) Get(id uint32) (*Userhasvideo, int, string, error) {
	model := &Userhasvideo{}
	err := m.DB.QueryRow(getUserhasvideoSQL, id).Scan(&model.Id, &model.UserId, &model.VideoId)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a userHasVideo with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get userHasVideo from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *UserhasvideoManager) List() (*[]Userhasvideo, int, string, error) {
	models := []Userhasvideo{}
	rows, err := m.DB.Query(listUserhasvideoSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get userHasVideos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Userhasvideo{}
		err = rows.Scan(&model.Id, &model.UserId, &model.VideoId)
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

func (m *UserhasvideoManager) CustomList(sql string) (*[]Userhasvideo, int, string, error) {
	models := []Userhasvideo{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get userHasVideos from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Userhasvideo{}
		err = rows.Scan(&model.Id, &model.UserId, &model.VideoId)
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

func (m *UserhasvideoManager) Update(model *Userhasvideo) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateUserhasvideoSQL, model.UserId, model.VideoId, model.Id)
	if err != nil {
		msg := "Couldn't update userHasVideo in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserhasvideoManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteUserhasvideoSQL, id)
	if err != nil {
		msg := "Couldn't delete userHasVideo in database"
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
