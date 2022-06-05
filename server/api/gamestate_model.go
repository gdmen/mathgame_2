// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateGamestateTableSQL = `
    CREATE TABLE gamestates (
        user_id BIGINT UNSIGNED PRIMARY KEY,
	problem_id BIGINT UNSIGNED NOT NULL,
	num_solved INT(5) NOT NULL,
	num_target INT(5) NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createGamestateSQL = `INSERT INTO gamestates (user_id, problem_id, num_solved, num_target) VALUES (?, ?, ?, ?);`

	getGamestateSQL = `SELECT * FROM gamestates WHERE user_id=?;`

	getGamestateKeySQL = `SELECT  FROM gamestates WHERE user_id=?, problem_id=?, num_solved=?, num_target=?;`

	listGamestateSQL = `SELECT * FROM gamestates;`

	updateGamestateSQL = `UPDATE gamestates SET problem_id=?, num_solved=?, num_target=? WHERE user_id=?;`

	deleteGamestateSQL = `DELETE FROM gamestates WHERE user_id=?;`
)

type Gamestate struct {
	UserId    uint64 `json:"user_id" uri:"user_id"`
	ProblemId uint64 `json:"problem_id" uri:"problem_id" form:"problem_id"`
	NumSolved int    `json:"num_solved" uri:"num_solved" form:"num_solved"`
	NumTarget int    `json:"num_target" uri:"num_target" form:"num_target"`
}

func (model Gamestate) String() string {
	return fmt.Sprintf("UserId: %v, ProblemId: %v, NumSolved: %v, NumTarget: %v", model.UserId, model.ProblemId, model.NumSolved, model.NumTarget)
}

type GamestateManager struct {
	DB *sql.DB
}

func (m *GamestateManager) Create(model *Gamestate) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createGamestateSQL, model.UserId, model.ProblemId, model.NumSolved, model.NumTarget)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add gamestate to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *GamestateManager) Get(user_id uint64) (*Gamestate, int, string, error) {
	model := &Gamestate{}
	err := m.DB.QueryRow(getGamestateSQL, user_id).Scan(&model.UserId, &model.ProblemId, &model.NumSolved, &model.NumTarget)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a gamestate with that user_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get gamestate from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *GamestateManager) List() (*[]Gamestate, int, string, error) {
	models := []Gamestate{}
	rows, err := m.DB.Query(listGamestateSQL)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get gamestates from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Gamestate{}
		err = rows.Scan(&model.UserId, &model.ProblemId, &model.NumSolved, &model.NumTarget)
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

func (m *GamestateManager) Update(model *Gamestate) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.UserId)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateGamestateSQL, model.ProblemId, model.NumSolved, model.NumTarget, model.UserId)
	if err != nil {
		msg := "Couldn't update gamestate in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *GamestateManager) Delete(user_id uint64) (int, string, error) {
	result, err := m.DB.Exec(deleteGamestateSQL, user_id)
	if err != nil {
		msg := "Couldn't delete gamestate in database"
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
