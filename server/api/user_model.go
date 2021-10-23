// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/volatiletech/authboss/v3"
)

var (
	assertUser   = &User{}
	assertStorer = &UserManager{}

	_ authboss.User         = assertUser
	_ authboss.AuthableUser = assertUser

	_ authboss.CreatingServerStorer = assertStorer
)

const (
	CreateUserTableSQL = `
	CREATE TABLE users (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(320) CHARACTER SET utf8 COLLATE utf8_general_ci UNIQUE NOT NULL,
		password VARCHAR(225) CHARACTER SET utf8 COLLATE utf8_general_ci NOT NULL,
		name VARCHAR(128) CHARACTER SET utf8 COLLATE utf8_general_ci NOT NULL
	);`
	createUserSQL = `
	INSERT INTO users(email, password, name) VALUES(?, ?, ?);`
	updateUserSQL = `
	UPDATE users SET password=?, name=? WHERE email=?;`
	getUserSQL = `
	SELECT * FROM users WHERE email=?;`
)

type User struct {
	Id       uint64 `json:"id"`
	Email    string `json:"email" form:"email"`
	Password string `json:"password" form:"password"`
	Name     string `json:"name" form:"name"`
}

func (model User) String() string {
	return fmt.Sprintf("Id: %d, Email: %s, Password: %s, Name: %s", model.Id, model.Email, model.Password, model.Name)
}

func (model *User) GetPID() (pid string) {
	return model.Email
}
func (model *User) PutPID(pid string) {
	model.Email = pid
}
func (model *User) GetPassword() (password string) {
	return model.Password
}
func (model *User) PutPassword(password string) {
	model.Password = password
}

type UserManager struct {
	DB *sql.DB
}

// New creates a blank user, it is not yet persisted in the database
// but is just for storing data
func (m *UserManager) New(ctx context.Context) authboss.User {
	return &User{}
}

// Create the user in storage, it should not overwrite a user
// and should return ErrUserFound if it currently exists.
func (m *UserManager) Create(ctx context.Context, user authboss.User) error {
	model := user.(*User)
	_, err := m.DB.Exec(createUserSQL, model.Email, model.Password, model.Name)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return authboss.ErrUserFound
		}
		return err
	}
	return nil
}

// Load will look up the user based on the passed in PrimaryID
func (m *UserManager) Load(ctx context.Context, id string) (authboss.User, error) {
	model := &User{}
	err := m.DB.QueryRow(getUserSQL, id).Scan(&model.Id, &model.Email, &model.Password, &model.Name)
	if err == sql.ErrNoRows {
		return nil, authboss.ErrUserNotFound
	} else if err != nil {
		return nil, err
	}
	return model, nil
}

// Save persists the user in the database, this should never
// create a user and instead return ErrUserNotFound if the user
// does not exist.
func (m *UserManager) Save(ctx context.Context, user authboss.User) error {
	model := user.(*User)
	// Check for existence
	user, err := m.Load(ctx, model.Email)
	if err != nil {
		return err
	}
	// Update
	_, err = m.DB.Exec(updateUserSQL, model.Email, model.Password, model.Name, model.Id)
	return err
}
