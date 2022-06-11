// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateProblemTableSQL = `
    CREATE TABLE problems (
        id BIGINT UNSIGNED PRIMARY KEY UNIQUE,
	expression TEXT NOT NULL,
	answer TEXT NOT NULL,
	difficulty FLOAT NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createProblemSQL = `INSERT INTO problems (id, expression, answer, difficulty) VALUES (?, ?, ?, ?);`

	getProblemSQL = `SELECT * FROM problems WHERE id=?;`

	getProblemKeySQL = `SELECT  FROM problems WHERE id=? AND expression=? AND answer=? AND difficulty=?;`

	listProblemSQL = `SELECT * FROM problems;`

	updateProblemSQL = `UPDATE problems SET expression=?, answer=?, difficulty=? WHERE id=?;`

	deleteProblemSQL = `DELETE FROM problems WHERE id=?;`
)

type Problem struct {
	Id         uint32  `json:"id" uri:"id"`
	Expression string  `json:"expression" uri:"expression" form:"expression"`
	Answer     string  `json:"answer" uri:"answer" form:"answer"`
	Difficulty float64 `json:"difficulty" uri:"difficulty" form:"difficulty"`
}

func (model Problem) String() string {
	return fmt.Sprintf("Id: %v, Expression: %v, Answer: %v, Difficulty: %v", model.Id, model.Expression, model.Answer, model.Difficulty)
}

type ProblemManager struct {
	DB *sql.DB
}

func (m *ProblemManager) Create(model *Problem) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createProblemSQL, model.Id, model.Expression, model.Answer, model.Difficulty)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add problem to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *ProblemManager) Get(id uint32) (*Problem, int, string, error) {
	model := &Problem{}
	err := m.DB.QueryRow(getProblemSQL, id).Scan(&model.Id, &model.Expression, &model.Answer, &model.Difficulty)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a problem with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get problem from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *ProblemManager) List() (*[]Problem, int, string, error) {
	models := []Problem{}
	rows, err := m.DB.Query(listProblemSQL)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get problems from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Problem{}
		err = rows.Scan(&model.Id, &model.Expression, &model.Answer, &model.Difficulty)
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

func (m *ProblemManager) Update(model *Problem) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateProblemSQL, model.Expression, model.Answer, model.Difficulty, model.Id)
	if err != nil {
		msg := "Couldn't update problem in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *ProblemManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteProblemSQL, id)
	if err != nil {
		msg := "Couldn't delete problem in database"
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
