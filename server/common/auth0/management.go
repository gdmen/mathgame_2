package auth0

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// httpClient is shared by the Management API calls below; the short timeout
// keeps a slow/unreachable Auth0 from blocking an account-deletion request.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// DeleteUser removes a user from Auth0 via the Management API using a
// client-credentials grant for the given machine-to-machine application.
//
// It is meant to be called best-effort: the caller has already scrubbed our
// own database, so a failure here is logged but not fatal. domain is the bare
// Auth0 tenant domain (e.g. "example.us.auth0.com"); auth0UserID is the user's
// `sub` (e.g. "auth0|abc123").
func DeleteUser(domain, clientID, clientSecret, auth0UserID string) error {
	token, err := managementToken(domain, clientID, clientSecret)
	if err != nil {
		return fmt.Errorf("get management token: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s/api/v2/users/%s", domain, url.PathEscape(auth0UserID))
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth0 delete user returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// managementToken fetches an access token for the Auth0 Management API
// (audience https://{domain}/api/v2/) via the client-credentials grant.
func managementToken(domain, clientID, clientSecret string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     clientID,
		"client_secret": clientSecret,
		"audience":      fmt.Sprintf("https://%s/api/v2/", domain),
	})
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("https://%s/oauth/token", domain)
	resp, err := httpClient.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("auth0 token response missing access_token")
	}
	return out.AccessToken, nil
}
