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
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"The Infra Podcast #313","slug":"the-infra-podcast-313"}`)))
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
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.Session.ID, nil))
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

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(`{"title":"Joinable","slug":"joinable"}`)))
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
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, controlIndexFile), []byte("<div id=\"root\"></div>"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	handler := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{ControlWebDistDir: distDir})

	for _, path := range []string{"/", "/sessions", "/sessions/sess_123", "/join/joinable?token=token"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d body = %s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "root") {
			t.Fatalf("GET %s body = %s", path, response.Body.String())
		}
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
	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil))), db
}
