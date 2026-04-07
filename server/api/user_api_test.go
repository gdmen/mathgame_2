package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"garydmenezes.com/mathgame/server/common"
)

func TestGetUser_ReturnsCreatedUser(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|get-user-test", "getuser@test.com", "getusertest")

	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(user.Auth0Id), user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	body, _ := ioutil.ReadAll(resp.Body)
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Auth0Id != user.Auth0Id {
		t.Errorf("auth0_id: want %s, got %s", user.Auth0Id, u.Auth0Id)
	}
	if u.Email != user.Email {
		t.Errorf("email: want %s, got %s", user.Email, u.Email)
	}
	if u.Username != user.Username {
		t.Errorf("username: want %s, got %s", user.Username, u.Username)
	}
}

func TestUpdateUser_ChangesFields(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0|update-user-test", "updateuser@test.com", "updateusertest")

	// Update via POST /users/:auth0_id
	updated := User{
		Auth0Id:  user.Auth0Id,
		Email:    "newemail@test.com",
		Username: "newusername",
	}
	body, _ := json.Marshal(updated)
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(user.Auth0Id), user.Auth0Id), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("POST user update: expected %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.Bytes())
	}

	// Verify via GET
	resp = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(user.Auth0Id), user.Auth0Id), nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET user: expected %d, got %d", http.StatusOK, resp.Code)
	}
	body, _ = ioutil.ReadAll(resp.Body)
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Email != "newemail@test.com" {
		t.Errorf("email: want newemail@test.com, got %s", u.Email)
	}
	if u.Username != "newusername" {
		t.Errorf("username: want newusername, got %s", u.Username)
	}
}
