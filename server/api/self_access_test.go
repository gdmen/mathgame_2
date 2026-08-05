package api // import "garydmenezes.com/mathgame/server/api"

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"garydmenezes.com/mathgame/server/common"
)

// knownPathParams is every path param the route surface may use that does NOT
// name a user. A param that names a user belongs in selfOnlyPathParams instead;
// a param in neither list fails TestSelfOnly_NoUnknownPathParams, so the "is
// this a user id?" question gets asked exactly once, when the route is
// registered — a route like /report/:student_id can't slip past RequireSelf by
// using a name the guard doesn't recognize.
var knownPathParams = map[string]bool{
	"id": true, "playlist_id": true, "seconds": true,
}

func TestSelfOnly_NoUnknownPathParams(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	_, r, cleanup := setupTestAPI(t, c)
	defer cleanup()

	selfOnly := make(map[string]bool, len(selfOnlyPathParams))
	for _, p := range selfOnlyPathParams {
		selfOnly[p] = true
	}
	for _, ri := range r.Routes() {
		for _, seg := range strings.Split(ri.Path, "/") {
			p, ok := strings.CutPrefix(seg, ":")
			if ok && !selfOnly[p] && !knownPathParams[p] {
				t.Errorf("%s %s: unknown path param :%s — if it names a user, add it to selfOnlyPathParams (RequireSelf and these tests then cover it); otherwise add it to knownPathParams", ri.Method, ri.Path, p)
			}
		}
	}
}

// selfOnlyRoutes reads the self-only route surface off the router itself rather
// than a hand-maintained list, so a newly registered route that names a user in
// its path is covered by these tests the moment it exists.
func selfOnlyRoutes(t *testing.T, r *gin.Engine) []gin.RouteInfo {
	t.Helper()
	var out []gin.RouteInfo
	for _, ri := range r.Routes() {
		for _, p := range selfOnlyPathParams {
			if strings.Contains(ri.Path, ":"+p) {
				out = append(out, ri)
				break
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no routes carry any of %v; these tests would pass vacuously", selfOnlyPathParams)
	}
	return out
}

// fillRoutePath substitutes a concrete request path for a gin route pattern,
// naming the given user wherever the pattern names one.
func fillRoutePath(t *testing.T, path string, u *User) string {
	t.Helper()
	path = strings.ReplaceAll(path, ":auth0_id", url.PathEscape(u.Auth0Id))
	path = strings.ReplaceAll(path, ":user_id", strconv.FormatUint(uint64(u.Id), 10))
	path = strings.ReplaceAll(path, ":seconds", "60")
	// An unsubstituted param would be requested literally and 404, which reads
	// as "the guard is missing" instead of "this test needs teaching".
	if strings.Contains(path, ":") {
		t.Fatalf("no substitution for a path param in %q: add one to fillRoutePath", path)
	}
	return path
}

func selfOnlyRequest(t *testing.T, r *gin.Engine, method, path, callerAuth0Id string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(method,
		fmt.Sprintf("%s?test_auth0_id=%s", path, url.QueryEscape(callerAuth0Id)),
		bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

// TestSelfOnly_OtherUsersIdIsForbidden is the regression test for the
// cross-account hole: every route that names a user in its path must 403 when
// the caller asks for somebody else's id.
func TestSelfOnly_OtherUsersIdIsForbidden(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	victim := createTestUser(t, r, "auth0id|selfvictim", "victim@test.com", "victim")
	attacker := createTestUser(t, r, "auth0id|selfattacker", "attacker@test.com", "attacker")
	// Provision the victim fully, or routes that 403 for their own reasons on an
	// empty account (GET /play/:user_id below the video floor) pass without the
	// guard.
	insertVideosAndUserHasVideo(t, api, victim.Id, minPlayableVideos)

	for _, ri := range selfOnlyRoutes(t, r) {
		t.Run(ri.Method+" "+ri.Path, func(t *testing.T) {
			path := fillRoutePath(t, ri.Path, victim)
			resp := selfOnlyRequest(t, r, ri.Method, path, attacker.Auth0Id)
			if resp.Code != http.StatusForbidden {
				t.Errorf("%s %s as another user: expected 403, got %d: %s",
					ri.Method, path, resp.Code, resp.Body.String())
			}
		})
	}
}

// TestSelfOnly_OwnIdStillAllowed keeps the guard honest: a blanket 403 would
// satisfy the test above.
func TestSelfOnly_OwnIdStillAllowed(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	user := createTestUser(t, r, "auth0id|selfowner", "owner@test.com", "owner")
	// GET /play/:user_id 403s on its own below the video floor.
	insertVideosAndUserHasVideo(t, api, user.Id, minPlayableVideos)

	gets := 0
	for _, ri := range selfOnlyRoutes(t, r) {
		if ri.Method != http.MethodGet {
			continue
		}
		gets++
		t.Run(ri.Method+" "+ri.Path, func(t *testing.T) {
			path := fillRoutePath(t, ri.Path, user)
			resp := selfOnlyRequest(t, r, ri.Method, path, user.Auth0Id)
			if resp.Code != http.StatusOK {
				t.Errorf("%s %s as self: expected 200, got %d: %s",
					ri.Method, path, resp.Code, resp.Body.String())
			}
		})
	}
	if gets == 0 {
		t.Fatal("no self-only GET routes found; this test would pass vacuously")
	}
}

// TestSelfOnly_CannotWriteAnotherUsersSettings pins the write half of the hole:
// customUpdateSettings binds user_id from the URI and passes it straight to the
// UPDATE, so an unguarded route lets one account reshape another's envelope.
func TestSelfOnly_CannotWriteAnotherUsersSettings(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	victim := createTestUser(t, r, "auth0id|setvictim", "setvictim@test.com", "setvictim")
	attacker := createTestUser(t, r, "auth0id|setattacker", "setattacker@test.com", "setattacker")

	before, status, msg, err := api.settingsManager.Get(victim.Id)
	if err != nil {
		t.Fatalf("read victim settings: %d %s %v", status, msg, err)
	}

	body := fmt.Sprintf(`{"user_id":%d,"problem_type_bitmap":3,"target_difficulty":9,"target_work_percentage":10}`, victim.Id)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", victim.Id, url.QueryEscape(attacker.Auth0Id)),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Errorf("expected 403 writing another user's settings, got %d: %s", resp.Code, resp.Body.String())
	}

	after, status, msg, err := api.settingsManager.Get(victim.Id)
	if err != nil {
		t.Fatalf("re-read victim settings: %d %s %v", status, msg, err)
	}
	if *after != *before {
		t.Errorf("victim settings changed: before %+v, after %+v", before, after)
	}
}

// TestSettingsWrite_IgnoresClientSuppliedUserId pins that customUpdateSettings
// picks the row it writes from the authenticated identity, not from either
// client-supplied channel. RequireSelf makes the path id safe on its own; this
// keeps the handler correct without depending on it.
func TestSettingsWrite_IgnoresClientSuppliedUserId(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	caller := createTestUser(t, r, "auth0id|bodycaller", "bodycaller@test.com", "bodycaller")
	other := createTestUser(t, r, "auth0id|bodyother", "bodyother@test.com", "bodyother")

	otherBefore, status, msg, err := api.settingsManager.Get(other.Id)
	if err != nil {
		t.Fatalf("read other settings: %d %s %v", status, msg, err)
	}

	// Own id in the path (so the guard passes), somebody else's in the body.
	body := fmt.Sprintf(`{"user_id":%d,"problem_type_bitmap":1,"target_difficulty":3,"target_work_percentage":60}`, other.Id)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/settings/%d?test_auth0_id=%s", caller.Id, url.QueryEscape(caller.Auth0Id)),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 writing own settings, got %d: %s", resp.Code, resp.Body.String())
	}

	callerAfter, status, msg, err := api.settingsManager.Get(caller.Id)
	if err != nil {
		t.Fatalf("read caller settings: %d %s %v", status, msg, err)
	}
	if callerAfter.TargetWorkPercentage != 60 {
		t.Errorf("caller's own row should have been written: %+v", callerAfter)
	}
	otherAfter, status, msg, err := api.settingsManager.Get(other.Id)
	if err != nil {
		t.Fatalf("re-read other settings: %d %s %v", status, msg, err)
	}
	if *otherAfter != *otherBefore {
		t.Errorf("body user_id steered the write: before %+v, after %+v", otherBefore, otherAfter)
	}
}

// TestUserWrite_IgnoresClientSuppliedAuth0Id pins the same property for
// customUpdateUser: the row it writes is chosen by the authenticated identity,
// so an auth0_id in the body cannot steer the update to another account.
func TestUserWrite_IgnoresClientSuppliedAuth0Id(t *testing.T) {
	c, err := common.ReadConfig("../../test_conf.json")
	if err != nil {
		t.Fatalf("Couldn't read config: %v", err)
	}
	api, r, cleanup := setupTestAPI(t, c)
	defer cleanup()
	caller := createTestUser(t, r, "auth0id|usrbodycaller", "usrbodycaller@test.com", "usrbodycaller")
	other := createTestUser(t, r, "auth0id|usrbodyother", "usrbodyother@test.com", "usrbodyother")

	// Own id in the path (so the guard passes), somebody else's in the body.
	body := fmt.Sprintf(`{"auth0_id":%q,"email":"stolen@test.com","username":"stolen","pin":"9999"}`, other.Auth0Id)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("/api/v1/users/%s?test_auth0_id=%s", url.PathEscape(caller.Auth0Id), url.QueryEscape(caller.Auth0Id)),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 updating own row, got %d: %s", resp.Code, resp.Body.String())
	}

	callerAfter, status, msg, err := api.userManager.Get(caller.Auth0Id)
	if err != nil {
		t.Fatalf("read caller row: %d %s %v", status, msg, err)
	}
	if callerAfter.Username != "stolen" || callerAfter.Pin != "9999" {
		t.Errorf("caller's own row should have been written: %+v", callerAfter)
	}
	otherAfter, status, msg, err := api.userManager.Get(other.Auth0Id)
	if err != nil {
		t.Fatalf("re-read other row: %d %s %v", status, msg, err)
	}
	if otherAfter.Username != "usrbodyother" || otherAfter.Email != "usrbodyother@test.com" || otherAfter.Pin != "" {
		t.Errorf("body auth0_id steered the write: %+v", otherAfter)
	}
}
