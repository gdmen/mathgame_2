// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreatePlaylistTableSQL = `
    CREATE TABLE playlists (
        id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
	you_tube_id VARCHAR(64) NOT NULL UNIQUE,
	title VARCHAR(512) NOT NULL DEFAULT '',
	thumbnailurl VARCHAR(1024) NOT NULL,
	etag VARCHAR(128) NOT NULL
    ) DEFAULT CHARSET=utf8mb4 ;`

	createPlaylistSQL = `INSERT INTO playlists (you_tube_id, thumbnailurl, etag) VALUES (?, ?, ?);`

	getPlaylistSQL = `SELECT * FROM playlists WHERE id=?;`

	getPlaylistKeySQL = `SELECT id FROM playlists WHERE you_tube_id=? AND thumbnailurl=? AND etag=?;`

	listPlaylistSQL = `SELECT * FROM playlists;`

	updatePlaylistSQL = `UPDATE playlists SET you_tube_id=?, title=?, thumbnailurl=?, etag=? WHERE id=?;`

	deletePlaylistSQL = `DELETE FROM playlists WHERE id=?;`
)

type Playlist struct {
	Id           uint32 `json:"id" uri:"id"`
	YouTubeId    string `json:"you_tube_id" uri:"you_tube_id" form:"you_tube_id"`
	Title        string `json:"title" uri:"title" form:"title"`
	ThumbnailURL string `json:"thumbnailurl" uri:"thumbnailurl" form:"thumbnailurl"`
	Etag         string `json:"etag" uri:"etag" form:"etag"`
}

func (model Playlist) String() string {
	return fmt.Sprintf("Id: %v, YouTubeId: %v, Title: %v, ThumbnailURL: %v, Etag: %v", model.Id, model.YouTubeId, model.Title, model.ThumbnailURL, model.Etag)
}

type PlaylistManager struct {
	DB *sql.DB
}

func (m *PlaylistManager) Create(model *Playlist) (int, string, error) {
	status := http.StatusCreated
	result, err := m.DB.Exec(createPlaylistSQL, model.YouTubeId, model.ThumbnailURL, model.Etag)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add playlist to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		err = m.DB.QueryRow(getPlaylistKeySQL, model.YouTubeId, model.ThumbnailURL, model.Etag).Scan(&model.Id)
		if err != nil {
			msg := "Couldn't add playlist to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	last_id, err := result.LastInsertId()
	if err != nil {
		msg := "Couldn't add playlist to database"
		return http.StatusInternalServerError, msg, err
	}
	model.Id = uint32(last_id)

	return status, "", nil
}

func (m *PlaylistManager) Get(id uint32) (*Playlist, int, string, error) {
	model := &Playlist{}
	err := m.DB.QueryRow(getPlaylistSQL, id).Scan(&model.Id, &model.YouTubeId, &model.Title, &model.ThumbnailURL, &model.Etag)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a playlist with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get playlist from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *PlaylistManager) List() (*[]Playlist, int, string, error) {
	models := []Playlist{}
	rows, err := m.DB.Query(listPlaylistSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get playlists from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Playlist{}
		err = rows.Scan(&model.Id, &model.YouTubeId, &model.Title, &model.ThumbnailURL, &model.Etag)
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

func (m *PlaylistManager) CustomList(sql string) (*[]Playlist, int, string, error) {
	models := []Playlist{}
	sql = "SELECT * FROM playlists WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get playlists from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Playlist{}
		err = rows.Scan(&model.Id, &model.YouTubeId, &model.Title, &model.ThumbnailURL, &model.Etag)
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

func (m *PlaylistManager) CustomIdList(sql string) (*[]uint32, int, string, error) {
	ids := []uint32{}
	sql = "SELECT id FROM playlists WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get playlists from database"
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

func (m *PlaylistManager) CustomSql(sql string) (int, string, error) {
	_, err := m.DB.Query(sql)
	if err != nil {
		msg := "Couldn't run sql for Playlist in database"
		return http.StatusBadRequest, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *PlaylistManager) Update(model *Playlist) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updatePlaylistSQL, model.YouTubeId, model.Title, model.ThumbnailURL, model.Etag, model.Id)
	if err != nil {
		msg := "Couldn't update playlist in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *PlaylistManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deletePlaylistSQL, id)
	if err != nil {
		msg := "Couldn't delete playlist in database"
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
