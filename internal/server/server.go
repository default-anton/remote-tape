package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/default-anton/remote-tape/internal/database"
)

type Server struct {
	db        *sql.DB
	logger    *slog.Logger
	startedAt time.Time
}

func New(db *sql.DB, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	srv := &Server{
		db:        db,
		logger:    logger,
		startedAt: time.Now().UTC(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.healthz)
	mux.HandleFunc("/readyz", srv.readyz)

	return requestLogger(logger, mux)
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
	w.Header().Set("Allow", method)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"ok":    false,
		"error": "method not allowed",
	})
	return false
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
