// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/internal/api"

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"

	"garydmenezes.com/mathgame/internal/generator"
)

const (
	CreateProblemTableSQL = `
	CREATE TABLE problems (
		id BIGINT(64) UNSIGNED NOT NULL PRIMARY KEY,
		expression TEXT CHARACTER SET utf8 COLLATE utf8_bin NOT NULL,
		answer TEXT CHARACTER SET utf8 COLLATE utf8_bin NOT NULL,
		difficulty FLOAT NOT NULL
	);`
	createProblemSQL = `
	INSERT INTO problems(id, expression, answer, difficulty) VALUES(?, ?, ?, ?);`
	deleteProblemSQL = `
	DELETE FROM problems WHERE id=?;`
	getProblemSQL = `
	SELECT * FROM problems WHERE id=?;`
	listProblemSQL = `
	SELECT * FROM problems;`
)

type Problem struct {
	Id   uint64  `json:"id" form:"id"`
	Expr string  `json:"expression" form:"expression"`
	Ans  string  `json:"answer" form:"answer"`
	Diff float64 `json:"difficulty" form:"difficulty"`
}

func (model Problem) String() string {
	return fmt.Sprintf("Id: %d, Expr: %s, Ans: %s, Diff: %f", model.Id, model.Expr, model.Ans, model.Diff)
}

type ProblemManager struct {
	DB *sql.DB
}

func (m *ProblemManager) Create(opts *generator.Options) (*Problem, int, string, error) {
	model := &Problem{}
	var err error
	model.Expr, model.Ans, model.Diff, err = generator.GenerateProblem(opts)
	if err != nil {
		if err, ok := err.(*generator.OptionsError); ok {
			msg := "Failed options validation"
			return nil, http.StatusBadRequest, msg, err
		}
		msg := "Couldn't generate problem"
		return nil, http.StatusBadRequest, msg, err
	}

	// Generate model.Hash
	h := fnv.New64a()
	h.Write([]byte(model.Expr))
	model.Id = h.Sum64()

	_, err = m.DB.Exec(createProblemSQL, model.Id, model.Expr, model.Ans, model.Diff)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add problem to database"
			return nil, http.StatusInternalServerError, msg, err
		}
		return model, http.StatusOK, "", nil
	}
	return model, http.StatusCreated, "", nil
}

func (m *ProblemManager) Delete(id uint64) (int, string, error) {
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

func (m *ProblemManager) Get(id uint64) (*Problem, int, string, error) {
	model := &Problem{}
	err := m.DB.QueryRow(getProblemSQL, id).Scan(&model.Id, &model.Expr, &model.Ans, &model.Diff)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a problem with that Id"
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
		err = rows.Scan(&model.Id, &model.Expr, &model.Ans, &model.Diff)
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

func (m *ProblemManager) Custom(sql string) (*[]Problem, int, string, error) {
	models := []Problem{}
	rows, err := m.DB.Query(sql)
	defer rows.Close()
	if err != nil {
		msg := "Couldn't get problems from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Problem{}
		err = rows.Scan(&model.Id, &model.Expr, &model.Ans, &model.Diff)
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
