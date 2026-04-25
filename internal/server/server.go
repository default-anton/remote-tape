package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/default-anton/remote-tape/internal/controlui"
	"github.com/default-anton/remote-tape/internal/database"
	"github.com/default-anton/remote-tape/internal/session"
)

type Options struct {
	ControlPlaneURL    string
	SessionsBaseDomain string
	DefaultRegion      string
	DefaultDropletSize string
	ImageID            string
	ControlWebDistDir  string
}

type Server struct {
	db        *sql.DB
	repo      *session.Repository
	logger    *slog.Logger
	startedAt time.Time
	options   Options
	controlUI fs.FS
}

func New(db *sql.DB, logger *slog.Logger, options ...Options) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	opts := defaultOptions()
	if len(options) > 0 {
		opts = mergeOptions(opts, options[0])
	}
	srv := &Server{
		db:        db,
		repo:      session.NewRepository(db),
		logger:    logger,
		startedAt: time.Now().UTC(),
		options:   opts,
		controlUI: controlUIFS(opts),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.app)
	mux.HandleFunc("/healthz", srv.healthz)
	mux.HandleFunc("/readyz", srv.readyz)
	mux.HandleFunc("/api/sessions", srv.apiSessions)
	mux.HandleFunc("/api/sessions/", srv.apiSession)
	mux.HandleFunc("/api/join/", srv.apiJoin)

	return requestLogger(logger, mux)
}

func defaultOptions() Options {
	return Options{
		ControlPlaneURL:    "http://127.0.0.1:8080",
		SessionsBaseDomain: "sessions.localhost",
		DefaultRegion:      "nyc3",
		DefaultDropletSize: "s-2vcpu-2gb",
		ImageID:            "ubuntu-24-04-x64",
	}
}

func mergeOptions(base Options, override Options) Options {
	if override.ControlPlaneURL != "" {
		base.ControlPlaneURL = override.ControlPlaneURL
	}
	if override.SessionsBaseDomain != "" {
		base.SessionsBaseDomain = override.SessionsBaseDomain
	}
	if override.DefaultRegion != "" {
		base.DefaultRegion = override.DefaultRegion
	}
	if override.DefaultDropletSize != "" {
		base.DefaultDropletSize = override.DefaultDropletSize
	}
	if override.ImageID != "" {
		base.ImageID = override.ImageID
	}
	if override.ControlWebDistDir != "" {
		base.ControlWebDistDir = override.ControlWebDistDir
	}
	return base
}

func controlUIFS(opts Options) fs.FS {
	if opts.ControlWebDistDir != "" {
		return os.DirFS(opts.ControlWebDistDir)
	}
	return controlui.FS()
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    "remote-tape-control",
		"started_at": s.startedAt.Format(time.RFC3339Nano),
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := http.StatusOK
	body := map[string]any{
		"ok":       true,
		"database": "ok",
	}
	if err := s.db.PingContext(ctx); err != nil {
		status = http.StatusServiceUnavailable
		body["ok"] = false
		body["database"] = "error"
		body["error"] = err.Error()
		writeJSON(w, status, body)
		return
	}
	count, err := database.AppliedMigrationCount(ctx, s.db)
	if err != nil {
		status = http.StatusServiceUnavailable
		body["ok"] = false
		body["database"] = "error"
		body["error"] = err.Error()
		writeJSON(w, status, body)
		return
	}
	body["migrations_applied"] = count
	writeJSON(w, status, body)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	writeMethodNotAllowed(w, method)
	return false
}

func writeMethodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message})
}

func writeOperationError(w http.ResponseWriter, status int, operation string, err error) {
	writeJSON(w, status, map[string]any{
		"ok":        false,
		"error":     operation + " failed",
		"operation": operation,
		"detail":    err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}

		logger.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds(),
			"remote_addr", clientIP(r),
		)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
