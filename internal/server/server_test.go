package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
