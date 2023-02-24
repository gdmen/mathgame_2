// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateSettingsTableSQL = `
    CREATE TABLE settings (
        user_id BIGINT UNSIGNED PRIMARY KEY,
	operations VARCHAR(256) NOT NULL DEFAULT '+',
	fractions TINYINT NOT NULL DEFAULT 0,
	negatives TINYINT NOT NULL DEFAULT 0,
	target_difficulty DOUBLE NOT NULL DEFAULT 3
    ) DEFAULT CHARSET=utf8 ;`

	createSettingsSQL = `INSERT INTO settings (user_id) VALUES (?);`

	getSettingsSQL = `SELECT * FROM settings WHERE user_id=?;`

	getSettingsKeySQL = `SELECT  FROM settingss WHERE user_id=?;`

	listSettingsSQL = `SELECT * FROM settings;`

	updateSettingsSQL = `UPDATE settings SET operations=?, fractions=?, negatives=?, target_difficulty=? WHERE user_id=?;`

	deleteSettingsSQL = `DELETE FROM settings WHERE user_id=?;`
)

type Settings struct {
	UserId           uint32  `json:"user_id" uri:"user_id"`
	Operations       string  `json:"operations" uri:"operations" form:"operations"`
	Fractions        bool    `json:"fractions" uri:"fractions" form:"fractions"`
	Negatives        bool    `json:"negatives" uri:"negatives" form:"negatives"`
	TargetDifficulty float64 `json:"target_difficulty" uri:"target_difficulty" form:"target_difficulty"`
}

func (model Settings) String() string {
	return fmt.Sprintf("UserId: %v, Operations: %v, Fractions: %v, Negatives: %v, TargetDifficulty: %v", model.UserId, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty)
}

type SettingsManager struct {
	DB *sql.DB
}

func (m *SettingsManager) Create(model *Settings) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createSettingsSQL, model.UserId)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add settings to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *SettingsManager) Get(user_id uint32) (*Settings, int, string, error) {
	model := &Settings{}
	err := m.DB.QueryRow(getSettingsSQL, user_id).Scan(&model.UserId, &model.Operations, &model.Fractions, &model.Negatives, &model.TargetDifficulty)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a settings with that user_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get settings from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *SettingsManager) List() (*[]Settings, int, string, error) {
	models := []Settings{}
	rows, err := m.DB.Query(listSettingsSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get settings from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Settings{}
		err = rows.Scan(&model.UserId, &model.Operations, &model.Fractions, &model.Negatives, &model.TargetDifficulty)
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

func (m *SettingsManager) CustomList(sql string) (*[]Settings, int, string, error) {
	models := []Settings{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get settings from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Settings{}
		err = rows.Scan(&model.UserId, &model.Operations, &model.Fractions, &model.Negatives, &model.TargetDifficulty)
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

func (m *SettingsManager) Update(model *Settings) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.UserId)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateSettingsSQL, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty, model.UserId)
	if err != nil {
		msg := "Couldn't update settings in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *SettingsManager) Delete(user_id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteSettingsSQL, user_id)
	if err != nil {
		msg := "Couldn't delete settings in database"
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
