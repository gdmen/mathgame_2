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
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		auth0_id VARCHAR(225) CHARACTER SET utf8 COLLATE utf8_general_ci UNIQUE NOT NULL,
		email VARCHAR(320) CHARACTER SET utf8 COLLATE utf8_general_ci NOT NULL,
		username VARCHAR(128) CHARACTER SET utf8 COLLATE utf8_general_ci NOT NULL
	);`
	createUserSQL = `
	INSERT INTO users(auth0_id, email, username) VALUES(?, ?, ?);`
	updateUserSQL = `
	UPDATE users SET email=?, username=? WHERE auth0_id=?;`
	getUserSQL = `
	SELECT * FROM users WHERE auth0_id=?;`
	getUserIdSQL = `
        SELECT id FROM users WHERE auth0_id=?;`
)

type User struct {
	Id       uint64 `json:"id"`
	Auth0Id  string `json:"auth0_id" form:"auth0_id"`
	Email    string `json:"email" form:"email"`
	Username string `json:"username" form:"username"`
}

func (model User) String() string {
	return fmt.Sprintf("Id: %d, Auth0Id: %s, Email: %s, Username: %s", model.Id, model.Auth0Id, model.Email, model.Username)
}

type UserManager struct {
	DB *sql.DB
}

func (m *UserManager) Create(model *User) (int, string, error) {
	_, err := m.DB.Exec(createUserSQL, model.Auth0Id, model.Email, model.Username)
	if err != nil {
		if !strings.Contains(err.Error(), "Duplicate entry") {
			msg := "Couldn't add user to database"
			return http.StatusInternalServerError, msg, err
		}
		// Get the id of the already existing model
		err = m.DB.QueryRow(getUserIdSQL, model.Auth0Id).Scan(&model.Id)
		if err != nil {
			panic("This should be impossible 1.")
		}
		return http.StatusOK, "", nil
	}
	// Get the id of the already existing model
	err = m.DB.QueryRow(getUserIdSQL, model.Auth0Id).Scan(&model.Id)
	if err != nil {
		panic("This should be impossible 2.")
	}
	return http.StatusCreated, "", nil
}

func (m *UserManager) Update(model *User) (int, string, error) {
	// Check for 404s
	_, status, msg, err := m.Get(model.Auth0Id)
	if err != nil {
		return status, msg, err
	}
	// Update
	_, err = m.DB.Exec(updateUserSQL, model.Email, model.Username, model.Auth0Id)
	if err != nil {
		msg := "Couldn't update user in database"
		return http.StatusInternalServerError, msg, err
	}
	return http.StatusOK, "", nil
}

func (m *UserManager) Get(auth0_id string) (*User, int, string, error) {
	model := &User{}
	err := m.DB.QueryRow(getUserSQL, auth0_id).Scan(&model.Id, &model.Auth0Id, &model.Email, &model.Username)
	if err == sql.ErrNoRows {
		msg := "Couldn't find a user with that auth0_id"
		return nil, http.StatusNotFound, msg, err
	} else if err != nil {
		msg := "Couldn't get user from database"
		return nil, http.StatusInternalServerError, msg, err
	}
	return model, http.StatusOK, "", nil
}
