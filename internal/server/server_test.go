package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/default-anton/remote-tape/internal/auth"
	"github.com/default-anton/remote-tape/internal/database"
)

func TestHealthz(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v", body["ok"])
	}
	if body["service"] != "remote-tape-control" {
		t.Fatalf("service = %v", body["service"])
	}
}

func TestReadyz(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v", body["ok"])
	}
	if body["database"] != "ok" {
		t.Fatalf("database = %v", body["database"])
	}
	if body["migrations_applied"] != float64(1) {
		t.Fatalf("migrations_applied = %v", body["migrations_applied"])
	}
}

func TestHealthzRejectsUnsafeMethod(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q", response.Header().Get("Allow"))
	}
}

func TestReadyzReportsDatabaseFailure(t *testing.T) {
	handler, db := newTestHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != false {
		t.Fatalf("ok = %v", body["ok"])
	}
}

func TestCreateAndGetSessionAPI(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"The Infra Podcast #313","slug":"the-infra-podcast-313"}`))
	addCookies(request, cookies)
	request.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var created struct {
		Session struct {
			ID         string  `json:"id"`
			Status     string  `json:"status"`
			RoomDomain *string `json:"room_domain"`
		} `json:"session"`
		JoinLinks map[string]struct {
			URL string `json:"url"`
		} `json:"join_links"`
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
		Tokens map[string]struct {
			Token string `json:"token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Session.ID == "" || created.Session.Status != "created" {
		t.Fatalf("created session = %+v", created.Session)
	}
	if created.Session.RoomDomain == nil || !strings.HasSuffix(*created.Session.RoomDomain, ".sessions.localhost") {
		t.Fatalf("room_domain = %v", created.Session.RoomDomain)
	}
	roomLabel := strings.TrimSuffix(*created.Session.RoomDomain, ".sessions.localhost")
	if !strings.HasPrefix(roomLabel, "room-") || len(roomLabel) > 63 || strings.Contains(roomLabel, "_") {
		t.Fatalf("room_domain label = %q", roomLabel)
	}
	if created.JoinLinks["host"].URL == "" || created.JoinLinks["guest"].URL == "" {
		t.Fatalf("join links = %+v", created.JoinLinks)
	}
	if created.Tokens["host"].Token == "" || created.Tokens["guest"].Token == "" {
		t.Fatalf("tokens = %+v", created.Tokens)
	}
	if len(created.Events) != 1 || created.Events[0].Type != "session.created" {
		t.Fatalf("events = %+v", created.Events)
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.Session.ID, nil)
	addCookies(getReq, cookies)
	handler.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", get.Code, get.Body.String())
	}
	if strings.Contains(get.Body.String(), created.Tokens["host"].Token) || strings.Contains(get.Body.String(), created.Tokens["guest"].Token) {
		t.Fatalf("GET response leaked raw join token: %s", get.Body.String())
	}
}

func TestJoinAPIValidatesToken(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()
	cookies, csrf := loginTestAdmin(t, handler)

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"Joinable","slug":"joinable"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}
	var created struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Tokens map[string]struct {
			Token string `json:"token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if _, err := db.ExecContext(context.Background(), `
update sessions
set droplet_id = 'do-123', droplet_ip = '203.0.113.10', dns_record_id = 'cf-123', livekit_url = 'wss://livekit.example.com', recording_download_url = 'https://recordings.example.com/session.zip', finalization_summary_json = '{"files":1}', last_error = 'operator diagnostics only'
where id = ?;
`, created.Session.ID); err != nil {
		t.Fatalf("seed admin-only session fields: %v", err)
	}

	join := httptest.NewRecorder()
	handler.ServeHTTP(join, httptest.NewRequest(http.MethodGet, "/api/join/joinable?token="+created.Tokens["guest"].Token, nil))
	if join.Code != http.StatusOK {
		t.Fatalf("join status = %d body = %s", join.Code, join.Body.String())
	}
	joinBody := join.Body.String()
	if !strings.Contains(joinBody, `"role":"guest"`) || strings.Contains(joinBody, created.Tokens["guest"].Token) {
		t.Fatalf("join response invalid or leaked raw token: %s", joinBody)
	}
	for _, forbidden := range []string{"image_id", "droplet_id", "droplet_ip", "dns_record_id", "livekit_url", "recording_download_url", "finalization_summary_json", "last_error", "access_tokens", "session_id"} {
		if strings.Contains(joinBody, forbidden) {
			t.Fatalf("join response leaked %q: %s", forbidden, joinBody)
		}
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/join/joinable?token=wrong", nil))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid status = %d", invalid.Code)
	}
}

func TestControlAppServesBuiltIndex(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile: "<div id=\"root\"></div>",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	for _, path := range []string{"/", "/sessions", "/sessions/sess_123", "/join/joinable?token=token", "/settings"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		addCookies(request, cookies)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body = %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("GET %s Cache-Control = %q", path, response.Header().Get("Cache-Control"))
		}
		if !strings.Contains(response.Body.String(), "root") {
			t.Fatalf("GET %s body = %s", path, response.Body.String())
		}
	}
}

func TestControlAppServesAssetsWithCacheHeaders(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile:       "<div id=\"root\"></div>",
		"assets/app.123abc.js": "console.log('remote-tape')",
		"favicon.ico":          "icon",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	asset := httptest.NewRecorder()
	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.123abc.js", nil)
	addCookies(assetReq, cookies)
	handler.ServeHTTP(asset, assetReq)
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d body = %s", asset.Code, asset.Body.String())
	}
	if asset.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", asset.Header().Get("Cache-Control"))
	}

	favicon := httptest.NewRecorder()
	faviconReq := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	addCookies(faviconReq, cookies)
	handler.ServeHTTP(favicon, faviconReq)
	if favicon.Code != http.StatusOK {
		t.Fatalf("favicon status = %d body = %s", favicon.Code, favicon.Body.String())
	}
	if favicon.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("favicon Cache-Control = %q", favicon.Header().Get("Cache-Control"))
	}
}

func TestControlAppDoesNotFallbackForAPIOrInvalidStaticPaths(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, map[string]string{
		controlIndexFile: "<div id=\"root\"></div>",
	})
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	api := httptest.NewRecorder()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	addCookies(apiReq, cookies)
	handler.ServeHTTP(api, apiReq)
	if api.Code != http.StatusNotFound {
		t.Fatalf("api status = %d body = %s", api.Code, api.Body.String())
	}

	asset := httptest.NewRecorder()
	assetReq := httptest.NewRequest(http.MethodGet, "/assets/%2e%2e/index.control.html", nil)
	addCookies(assetReq, cookies)
	handler.ServeHTTP(asset, assetReq)
	if asset.Code != http.StatusNotFound {
		t.Fatalf("asset status = %d body = %s", asset.Code, asset.Body.String())
	}
}

func TestControlAppReportsMissingBuild(t *testing.T) {
	handler, db := newTestHandlerWithControlDist(t, nil)
	defer db.Close()
	cookies, _ := loginTestAdmin(t, handler)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "pnpm --dir web build:control") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestDashboardAuthFlow(t *testing.T) {
	handler, db := newTestHandler(t)
	defer db.Close()

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	_, csrfCookie, csrf := csrfForTest(t, handler, nil)
	badLogin := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=wrong&csrf_token="+csrf))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(csrfCookie)
	handler.ServeHTTP(badLogin, badReq)
	if badLogin.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d", badLogin.Code)
	}

	cookies, csrf := loginTestAdmin(t, handler)
	if len(cookies) == 0 {
		t.Fatal("login did not set cookies")
	}

	authenticated := httptest.NewRecorder()
	authReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	addCookies(authReq, cookies)
	handler.ServeHTTP(authenticated, authReq)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d body = %s", authenticated.Code, authenticated.Body.String())
	}

	missingCSRF := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"No CSRF","slug":"no-csrf"}`))
	addCookies(missingReq, cookies)
	handler.ServeHTTP(missingCSRF, missingReq)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf status = %d", missingCSRF.Code)
	}

	created := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"With CSRF","slug":"with-csrf"}`))
	addCookies(createReq, cookies)
	createReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(created, createReq)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", created.Code, created.Body.String())
	}

	logout := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	addCookies(logoutReq, cookies)
	logoutReq.Header.Set("X-CSRF-Token", csrf)
	handler.ServeHTTP(logout, logoutReq)
	if logout.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d", logout.Code)
	}
	cleared := responseCookies(logout)["remote_tape_session"]
	if cleared == nil || cleared.MaxAge != -1 {
		t.Fatalf("logout did not clear session cookie: %#v", cleared)
	}

	postLogout := httptest.NewRecorder()
	postLogoutReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	for name, cookie := range cookies {
		if name != "remote_tape_session" {
			postLogoutReq.AddCookie(cookie)
		}
	}
	handler.ServeHTTP(postLogout, postLogoutReq)
	if postLogout.Code != http.StatusUnauthorized {
		t.Fatalf("post logout status = %d", postLogout.Code)
	}
}

func newTestHandler(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}
	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Auth: newTestAuth(t)}), db
}

func newTestHandlerWithControlDist(t *testing.T, files map[string]string) (http.Handler, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		db.Close()
		t.Fatalf("Migrate() error = %v", err)
	}
	controlUI := fstest.MapFS{}
	for name, content := range files {
		controlUI[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Auth: newTestAuth(t), controlUIFS: controlUI}), db
}

func newTestAuth(t *testing.T) *auth.Manager {
	t.Helper()
	hash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	manager, err := auth.NewManager(auth.Config{
		PasswordHash:         hash,
		CookieAuthKey:        []byte("0123456789abcdef0123456789abcdef"),
		CookieEncryptionKey:  []byte("0123456789abcdef0123456789abcdef"),
		SessionDuration:      time.Hour,
		RateLimitMaxAttempts: 5,
		RateLimitWindow:      15 * time.Minute,
		Logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func loginTestAdmin(t *testing.T, handler http.Handler) (map[string]*http.Cookie, string) {
	t.Helper()
	_, csrfCookie, csrf := csrfForTest(t, handler, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=correct-password&csrf_token="+csrf))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(csrfCookie)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d body = %s", response.Code, response.Body.String())
	}
	cookies := responseCookies(response)
	if cookies["remote_tape_session"] == nil {
		t.Fatalf("login missing session cookie: %#v", cookies)
	}
	_, csrfCookie, csrf = csrfForTest(t, handler, cookies)
	cookies[csrfCookie.Name] = csrfCookie
	return cookies, csrf
}

func csrfForTest(t *testing.T, handler http.Handler, cookies map[string]*http.Cookie) (map[string]*http.Cookie, *http.Cookie, string) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	addCookies(request, cookies)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("auth session status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode auth session: %v", err)
	}
	csrfCookie := responseCookies(response)["remote_tape_csrf"]
	if csrfCookie == nil || body.CSRFToken == "" {
		t.Fatalf("missing csrf cookie/token: cookie=%#v body=%s", csrfCookie, response.Body.String())
	}
	return responseCookies(response), csrfCookie, body.CSRFToken
}

func addCookies(request *http.Request, cookies map[string]*http.Cookie) {
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func responseCookies(response *httptest.ResponseRecorder) map[string]*http.Cookie {
	cookies := map[string]*http.Cookie{}
	for _, cookie := range response.Result().Cookies() {
		cookies[cookie.Name] = cookie
	}
	return cookies
}
