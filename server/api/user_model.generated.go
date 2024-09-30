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
        auth0_id VARCHAR(225) NOT NULL PRIMARY KEY,
	id BIGINT UNSIGNED AUTO_INCREMENT UNIQUE,
	email VARCHAR(320) NOT NULL,
	username VARCHAR(128) NOT NULL,
	pin VARCHAR(4) NOT NULL DEFAULT ''
    ) DEFAULT CHARSET=utf8mb4 ;`

	createUserSQL = `INSERT INTO users (auth0_id, email, username) VALUES (?, ?, ?);`

	getUserSQL = `SELECT * FROM users WHERE auth0_id=?;`

	getUserKeySQL = `SELECT id FROM users WHERE auth0_id=? AND email=? AND username=?;`

	listUserSQL = `SELECT * FROM users;`

	updateUserSQL = `UPDATE users SET id=?, email=?, username=?, pin=? WHERE auth0_id=?;`

	deleteUserSQL = `DELETE FROM users WHERE auth0_id=?;`
)

type User struct {
	Auth0Id  string `json:"auth0_id" uri:"auth0_id"`
	Id       uint32 `json:"id" uri:"id" form:"id"`
	Email    string `json:"email" uri:"email" form:"email"`
	Username string `json:"username" uri:"username" form:"username"`
	Pin      string `json:"pin" uri:"pin" form:"pin"`
}

func (model User) String() string {
	return fmt.Sprintf("Auth0Id: %v, Id: %v, Email: %v, Username: %v, Pin: %v", model.Auth0Id, model.Id, model.Email, model.Username, model.Pin)
}

type UserManager struct {
	DB *sql.DB
}

func (m *UserManager) Create(model *User) (int, string, error) {
	status := http.StatusCreated
	_, err := m.DB.Exec(createUserSQL, model.Auth0Id, model.Email, model.Username)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add user to database"
			return http.StatusInternalServerError, msg, err
		}

		// Update model with the configured return field.
		err = m.DB.QueryRow(getUserKeySQL, model.Auth0Id, model.Email, model.Username).Scan(&model.Id)
		if err != nil {
			msg := "Couldn't add user to database"
			return http.StatusInternalServerError, msg, err
		}

		return http.StatusOK, "", nil
	}

	return status, "", nil
}

func (m *UserManager) Get(auth0_id string) (*User, int, string, error) {
	model := &User{}
	err := m.DB.QueryRow(getUserSQL, auth0_id).Scan(&model.Auth0Id, &model.Id, &model.Email, &model.Username, &model.Pin)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a user with that auth0_id"
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
		err = rows.Scan(&model.Auth0Id, &model.Id, &model.Email, &model.Username, &model.Pin)
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
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get users from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		model := User{}
		err = rows.Scan(&model.Auth0Id, &model.Id, &model.Email, &model.Username, &model.Pin)
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

func (m *UserManager) CustomIdList(sql string) (*[]string, int, string, error) {
	ids := []string{}
	sql = "SELECT auth0_id FROM problems WHERE " + sql
	rows, err := m.DB.Query(sql)

	defer rows.Close()
	if err != nil {
		msg := "Couldn't get users from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	for rows.Next() {
		var auth0_id string
		err = rows.Scan(&auth0_id)
		if err != nil {
			msg := "Couldn't scan row from database"
			return nil, http.StatusInternalServerError, msg, err
		}
		ids = append(ids, auth0_id)
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
	_, status, msg, err := m.Get(model.Auth0Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateUserSQL, model.Id, model.Email, model.Username, model.Pin, model.Auth0Id)
	if err != nil {
		msg := "Couldn't update user in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserManager) Delete(auth0_id string) (int, string, error) {
	result, err := m.DB.Exec(deleteUserSQL, auth0_id)
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
