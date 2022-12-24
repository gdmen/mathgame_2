// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateUserhasteacherTableSQL = `
    CREATE TABLE userHasTeachers (
        id BIGINT UNSIGNED PRIMARY KEY UNIQUE,
	user_id BIGINT UNSIGNED NOT NULL,
	teacher_id BIGINT UNSIGNED NOT NULL
    ) DEFAULT CHARSET=utf8 ;`

	createUserhasteacherSQL = `INSERT INTO userHasTeachers (id, user_id, teacher_id) VALUES (?, ?, ?);`

	getUserhasteacherSQL = `SELECT * FROM userHasTeachers WHERE id=?;`

	getUserhasteacherKeySQL = `SELECT  FROM userHasTeachers WHERE id=? AND user_id=? AND teacher_id=?;`

	listUserhasteacherSQL = `SELECT * FROM userHasTeachers;`

	updateUserhasteacherSQL = `UPDATE userHasTeachers SET user_id=?, teacher_id=? WHERE id=?;`

	deleteUserhasteacherSQL = `DELETE FROM userHasTeachers WHERE id=?;`
)

type Userhasteacher struct {
	Id        uint32 `json:"id" uri:"id"`
	UserId    uint32 `json:"user_id" uri:"user_id" form:"user_id"`
	TeacherId uint32 `json:"teacher_id" uri:"teacher_id" form:"teacher_id"`
}

func (model Userhasteacher) String() string {
	return fmt.Sprintf("Id: %v, UserId: %v, TeacherId: %v", model.Id, model.UserId, model.TeacherId)
}

type UserhasteacherManager struct {
	DB *sql.DB
}

func (m *UserhasteacherManager) Create(model *Userhasteacher) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createUserhasteacherSQL, model.Id, model.UserId, model.TeacherId)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add userHasTeacher to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *UserhasteacherManager) Get(id uint32) (*Userhasteacher, int, string, error) {
	model := &Userhasteacher{}
	err := m.DB.QueryRow(getUserhasteacherSQL, id).Scan(&model.Id, &model.UserId, &model.TeacherId)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a userHasTeacher with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get userHasTeacher from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *UserhasteacherManager) List() (*[]Userhasteacher, int, string, error) {
	models := []Userhasteacher{}
	rows, err := m.DB.Query(listUserhasteacherSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get userHasTeachers from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Userhasteacher{}
		err = rows.Scan(&model.Id, &model.UserId, &model.TeacherId)
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

func (m *UserhasteacherManager) CustomList(sql string) (*[]Userhasteacher, int, string, error) {
	models := []Userhasteacher{}
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get userHasTeachers from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := Userhasteacher{}
		err = rows.Scan(&model.Id, &model.UserId, &model.TeacherId)
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

func (m *UserhasteacherManager) Update(model *Userhasteacher) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateUserhasteacherSQL, model.UserId, model.TeacherId, model.Id)
	if err != nil {
		msg := "Couldn't update userHasTeacher in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserhasteacherManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteUserhasteacherSQL, id)
	if err != nil {
		msg := "Couldn't delete userHasTeacher in database"
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
