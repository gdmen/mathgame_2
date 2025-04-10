// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

const (
	CreateUserTableSQL = `
    CREATE TABLE users (
        id BIGINT UNSIGNED PRIMARY KEY UNIQUE,
	email VARCHAR(320) NOT NULL UNIQUE,
	password TEXT,
	name VARCHAR(128) NOT NULL,
	pin VARCHAR(4) NOT NULL DEFAULT ''
    ) DEFAULT CHARSET=utf8mb4 ;`

	createUserSQL = `INSERT INTO users (id, email, password, name) VALUES (?, ?, ?, ?);`

	getUserSQL = `SELECT * FROM users WHERE id=?;`

	getUserKeySQL = `SELECT  FROM users WHERE id=? AND email=? AND password=? AND name=?;`

	listUserSQL = `SELECT * FROM users;`

	updateUserSQL = `UPDATE users SET email=?, password=?, name=?, pin=? WHERE id=?;`

	deleteUserSQL = `DELETE FROM users WHERE id=?;`
)

type User struct {
	Id       uint32 `json:"id" uri:"id"`
	Email    string `json:"email" uri:"email" form:"email"`
	Password string `json:"password" uri:"password" form:"password"`
	Name     string `json:"name" uri:"name" form:"name"`
	Pin      string `json:"pin" uri:"pin" form:"pin"`
}

func (model User) String() string {
	return fmt.Sprintf("Id: %v, Email: %v, Password: %v, Name: %v, Pin: %v", model.Id, model.Email, model.Password, model.Name, model.Pin)
}

type UserManager struct {
	DB *sql.DB
}

func (m *UserManager) Create(model *User) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createUserSQL, model.Id, model.Email, model.Password, model.Name)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add user to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *UserManager) Get(id uint32) (*User, int, string, error) {
	model := &User{}
	err := m.DB.QueryRow(getUserSQL, id).Scan(&model.Id, &model.Email, &model.Password, &model.Name, &model.Pin)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a user with that id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get user from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}

func (m *UserManager) List() (*[]User, int, string, error) {
	models := []User{}
	rows, err := m.DB.Query(listUserSQL)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get users from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := User{}
		err = rows.Scan(&model.Id, &model.Email, &model.Password, &model.Name, &model.Pin)
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

func (m *UserManager) CustomList(sql string) (*[]User, int, string, error) {
	models := []User{}
	sql = "SELECT * FROM users WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get users from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := User{}
		err = rows.Scan(&model.Id, &model.Email, &model.Password, &model.Name, &model.Pin)
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

func (m *UserManager) CustomIdList(sql string) (*[]uint32, int, string, error) {
	ids := []uint32{}
	sql = "SELECT id FROM users WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get users from database"
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

func (m *UserManager) CustomSql(sql string) (int, string, error) {
	_, err := m.DB.Query(sql)
	if err != nil {
		msg := "Couldn't run sql for User in database"
		return http.StatusBadRequest, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserManager) Update(model *User) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateUserSQL, model.Email, model.Password, model.Name, model.Pin, model.Id)
	if err != nil {
		msg := "Couldn't update user in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserManager) Delete(id uint32) (int, string, error) {
	result, err := m.DB.Exec(deleteUserSQL, id)
	if err != nil {
		msg := "Couldn't delete user in database"
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
