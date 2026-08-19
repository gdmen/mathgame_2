package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"garydmenezes.com/mathgame/server/common"
	"garydmenezes.com/mathgame/server/docs/spec"
)

// TestAdminGate verifies RequireAdmin on every route in the /admin group that
// has no body of its own to test elsewhere: a default (student) user is
// forbidden, a user whose role is admin is allowed.
func TestAdminGate(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	student := createTestUser(t, r, "auth0id|admin-student", "s@test.com", "student")
	admin := createTestAdmin(t, api, r, "auth0id|admin-admin", "a@test.com", "admin")

	get := func(user *User, path string) *httptest.ResponseRecorder {
		resp := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/v1/admin%s?test_auth0_id=%s", path, user.Auth0Id), nil)
		r.ServeHTTP(resp, req)
		return resp
	}

	for _, path := range []string{"/whoami", "/swagger.yaml"} {
		if resp := get(student, path); resp.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403 for student, got %d: %s", path, resp.Code, resp.Body.Bytes())
		}
	}

	t.Run("Whoami", func(t *testing.T) {
		resp := get(admin, "/whoami")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 for admin, got %d: %s", resp.Code, resp.Body.Bytes())
		}
		var body struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal whoami: %v", err)
		}
		if body.Role != RoleAdmin {
			t.Errorf("expected role %q, got %q", RoleAdmin, body.Role)
		}
	})

	t.Run("SwaggerSpec", func(t *testing.T) {
		resp := get(admin, "/swagger.yaml")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200 for admin, got %d: %s", resp.Code, resp.Body.Bytes())
		}
		if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
			t.Errorf("expected a YAML content type, got %q", ct)
		}
		if !bytes.Equal(resp.Body.Bytes(), spec.YAML) {
			t.Errorf("body is not the embedded spec")
		}
	})
}

// TestUpdateUser_CannotEscalateRole verifies that POSTing a role in the user
// update body does not change the stored role (no self-promotion), while the
// legitimate mutable field (pin) still updates.
func TestUpdateUser_CannotEscalateRole(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|no-escalate", "esc@test.com", "escalate")

	body, _ := json.Marshal(map[string]interface{}{
		"email":    user.Email,
		"username": user.Username,
		"pin":      "1234",
		"role":     RoleAdmin, // attempted escalation
	})
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(user.Auth0Id), user.Auth0Id),
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.Bytes())
	}

	var role, pin string
	if err := api.DB.QueryRow("SELECT role, pin FROM users WHERE auth0_id=?", user.Auth0Id).Scan(&role, &pin); err != nil {
		t.Fatalf("read back user: %v", err)
	}
	if role != RoleStudent {
		t.Errorf("role escalated: expected %q, got %q", RoleStudent, role)
	}
	if pin != "1234" {
		t.Errorf("legitimate field did not update: expected pin 1234, got %q", pin)
	}
}

// TestUpdateUser_CannotUpdateOtherUser verifies a caller cannot update a
// different user's row by passing someone else's auth0_id in the URL.
func TestUpdateUser_CannotUpdateOtherUser(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	attacker := createTestUser(t, r, "auth0id|attacker", "attacker@test.com", "attacker")
	victim := createTestUser(t, r, "auth0id|victim", "victim@test.com", "victim")

	// attacker (token) tries to update victim (URL) — expect 403.
	body, _ := json.Marshal(map[string]interface{}{
		"email":    "pwned@test.com",
		"username": victim.Username,
		"pin":      "9999",
	})
	resp := httptest.NewRecorder()
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(victim.Auth0Id), attacker.Auth0Id),
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 updating another user, got %d: %s", resp.Code, resp.Body.Bytes())
	}

	// victim's row is untouched.
	var email, pin string
	if err := api.DB.QueryRow("SELECT email, pin FROM users WHERE auth0_id=?", victim.Auth0Id).Scan(&email, &pin); err != nil {
		t.Fatalf("read back victim: %v", err)
	}
	if email != "victim@test.com" || pin != "" {
		t.Errorf("victim row was modified: email=%q pin=%q", email, pin)
	}
}
