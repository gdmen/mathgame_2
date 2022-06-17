// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateOptionTableSQL = `
    CREATE TABLE options (
        user_id BIGINT UNSIGNED PRIMARY KEY,
	operations VARCHAR(256) NOT NULL,
	fractions TINYINT NOT NULL,
	negatives TINYINT NOT NULL,
	target_difficulty DOUBLE NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createOptionSQL = `INSERT INTO options (user_id, operations, fractions, negatives, target_difficulty) VALUES (?, ?, ?, ?, ?);`

	getOptionSQL = `SELECT * FROM options WHERE user_id=?;`

	getOptionKeySQL = `SELECT  FROM options WHERE user_id=? AND operations=? AND fractions=? AND negatives=? AND target_difficulty=?;`

	listOptionSQL = `SELECT * FROM options;`

	updateOptionSQL = `UPDATE options SET operations=?, fractions=?, negatives=?, target_difficulty=? WHERE user_id=?;`

	deleteOptionSQL = `DELETE FROM options WHERE user_id=?;`
)

type Option struct {
	UserId           uint32  `json:"user_id" uri:"user_id"`
	Operations       string  `json:"operations" uri:"operations" form:"operations"`
	Fractions        bool    `json:"fractions" uri:"fractions" form:"fractions"`
	Negatives        bool    `json:"negatives" uri:"negatives" form:"negatives"`
	TargetDifficulty float64 `json:"target_difficulty" uri:"target_difficulty" form:"target_difficulty"`
}

func (model Option) String() string {
	return fmt.Sprintf("UserId: %v, Operations: %v, Fractions: %v, Negatives: %v, TargetDifficulty: %v", model.UserId, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty)
}

type OptionManager struct {
	DB *sql.DB
}

func (m *OptionManager) Create(model *Option) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createOptionSQL, model.UserId, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add option to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *OptionManager) Get(user_id uint32) (*Option, int, string, error) {
	model := &Option{}
	err := m.DB.QueryRow(getOptionSQL, user_id).Scan(&model.UserId, &model.Operations, &model.Fractions, &model.Negatives, &model.TargetDifficulty)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a option with that user_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get option from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *OptionManager) List() (*[]Option, int, string, error) {
	models := []Option{}
	rows, err := m.DB.Query(listOptionSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get options from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Option{}
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

func (m *OptionManager) CustomList(sql string) (*[]Option, int, string, error) {
	models := []Option{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get options from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Option{}
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

func (m *OptionManager) Update(model *Option) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.UserId)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateOptionSQL, model.Operations, model.Fractions, model.Negatives, model.TargetDifficulty, model.UserId)
	if err != nil {
		msg := "Couldn't update option in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *OptionManager) Delete(user_id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteOptionSQL, user_id)
	if err != nil {
		msg := "Couldn't delete option in database"
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
