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
	problem_type_bitmap BIGINT UNSIGNED NOT NULL,
	target_difficulty DOUBLE NOT NULL,
	target_work_percentage INT(3) NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createSettingsSQL = `INSERT INTO settings (user_id, problem_type_bitmap, target_difficulty, target_work_percentage) VALUES (?, ?, ?, ?);`

	getSettingsSQL = `SELECT * FROM settings WHERE user_id=?;`

	getSettingsKeySQL = `SELECT  FROM settingss WHERE user_id=? AND problem_type_bitmap=? AND target_difficulty=? AND target_work_percentage=?;`

	listSettingsSQL = `SELECT * FROM settings WHERE user_id=?;`

	updateSettingsSQL = `UPDATE settings SET problem_type_bitmap=?, target_difficulty=?, target_work_percentage=? WHERE user_id=?;`

	deleteSettingsSQL = `DELETE FROM settings WHERE user_id=?;`
)

type Settings struct {
	UserId               uint32  `json:"user_id" uri:"user_id"`
	ProblemTypeBitmap    uint64  `json:"problem_type_bitmap" uri:"problem_type_bitmap" form:"problem_type_bitmap"`
	TargetDifficulty     float64 `json:"target_difficulty" uri:"target_difficulty" form:"target_difficulty"`
	TargetWorkPercentage uint8   `json:"target_work_percentage" uri:"target_work_percentage" form:"target_work_percentage"`
}

func (model Settings) String() string {
	return fmt.Sprintf("UserId: %v, ProblemTypeBitmap: %v, TargetDifficulty: %v, TargetWorkPercentage: %v", model.UserId, model.ProblemTypeBitmap, model.TargetDifficulty, model.TargetWorkPercentage)
}

type SettingsManager struct {
	DB *sql.DB
}

func (m *SettingsManager) Create(model *Settings) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createSettingsSQL, model.UserId, model.ProblemTypeBitmap, model.TargetDifficulty, model.TargetWorkPercentage)
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
	err := m.DB.QueryRow(getSettingsSQL, user_id).Scan(&model.UserId, &model.ProblemTypeBitmap, &model.TargetDifficulty, &model.TargetWorkPercentage)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a settings with that user_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get settings from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *SettingsManager) List(user_id uint32) (*[]Settings, int, string, error) {
	models := []Settings{}
	rows, err := m.DB.Query(listSettingsSQL, user_id)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get settings from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Settings{}
		err = rows.Scan(&model.UserId, &model.ProblemTypeBitmap, &model.TargetDifficulty, &model.TargetWorkPercentage)
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
		err = rows.Scan(&model.UserId, &model.ProblemTypeBitmap, &model.TargetDifficulty, &model.TargetWorkPercentage)
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
	_, err = m.DB.Exec(updateSettingsSQL, model.ProblemTypeBitmap, model.TargetDifficulty, model.TargetWorkPercentage, model.UserId)
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
