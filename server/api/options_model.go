// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateOptionsTableSQL = `
    CREATE TABLE optionss (
        user_id BIGINT UNSIGNED PRIMARY KEY,
	operations VARCHAR(256) NOT NULL,
	fractions TINYINT NOT NULL,
	negatives TINYINT NOT NULL,
	target_difficulty DOUBLE NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createOptionsSQL = `INSERT INTO optionss (user_id, operations, fractions, negatives, target_difficulty) VALUES (?, ?, ?, ?, ?);`

	getOptionsSQL = `SELECT * FROM optionss WHERE user_id=?;`

	getOptionsKeySQL = `SELECT  FROM optionss WHERE user_id=?, operations=?, fractions=?, negatives=?, target_difficulty=?;`

	listOptionsSQL = `SELECT * FROM optionss;`

	updateOptionsSQL = `UPDATE optionss SET operations=?, fractions=?, negatives=?, target_difficulty=? WHERE user_id=?;`

	deleteOptionsSQL = `DELETE FROM optionss WHERE user_id=?;`
)

type Options struct {
	UserId           uint64  `json:"user_id" uri:"user_id"`
	Operations       string  `json:"operations" uri:"operations" form:"operations"`
	Fractions        bool    `json:"fractions" uri:"fractions" form:"fractions"`
	Negatives        bool    `json:"negatives" uri:"negatives" form:"negatives"`
	TargetDifficulty float64 `json:"target_difficulty" uri:"target_difficulty" form:"target_difficulty"`
}

func (model Options) String() string {
	return fmt.Sprintf("UserId: %v, Operations: %v, Fractions: %v, Negatives: %v, TargetDifficulty: %v", model.UserId, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty)
}

type OptionsManager struct {
	DB *sql.DB
}

func (m *OptionsManager) Create(model *Options) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createOptionsSQL, model.UserId, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add options to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *OptionsManager) Get(user_id uint64) (*Options, int, string, error) {
	model := &Options{}
	err := m.DB.QueryRow(getOptionsSQL, user_id).Scan(&model.UserId, &model.Operations, &model.Fractions, &model.Negatives, &model.TargetDifficulty)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a options with that user_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get options from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *OptionsManager) List() (*[]Options, int, string, error) {
	models := []Options{}
	rows, err := m.DB.Query(listOptionsSQL)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get optionss from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Options{}
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

func (m *OptionsManager) Update(model *Options) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.UserId)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateOptionsSQL, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty, model.UserId)
	if err != nil {
		msg := "Couldn't update options in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *OptionsManager) Delete(user_id uint64) (int, string, error) {
	result, err := m.DB.Exec(deleteOptionsSQL, user_id)
	if err != nil {
		msg := "Couldn't delete options in database"
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
