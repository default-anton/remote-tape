package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenAppliesSQLitePragmas(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	assertPragma(t, db, "foreign_keys", "1")
	assertPragma(t, db, "journal_mode", "wal")
	assertPragma(t, db, "busy_timeout", "5000")
	assertPragma(t, db, "synchronous", "1")
}

func TestOpenRestrictsDatabaseFilePermissions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data", "control-plane.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	assertPerm(t, filepath.Dir(path), 0o700)
	assertPerm(t, path, 0o600)
	for _, candidate := range []string{path + "-wal", path + "-shm"} {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			t.Fatalf("stat %q: %v", candidate, err)
		}
		assertPerm(t, candidate, 0o600)
	}
}

func TestMigrateCreatesInitialSchema(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	result, err := Migrate(ctx, db, discardLogger())
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if result.Current != 1 {
		t.Fatalf("Current = %d", result.Current)
	}
	if len(result.Applied) != 1 {
		t.Fatalf("Applied length = %d", len(result.Applied))
	}

	for _, table := range []string{"sessions", "session_access_tokens", "session_events", "schema_migrations"} {
		if !tableExists(t, db, table) {
			t.Fatalf("table %q does not exist", table)
		}
		assertStrictTable(t, db, table)
	}
	for table, columns := range map[string][]string{
		"sessions": {
			"created_at",
			"updated_at",
			"ready_at",
			"active_at",
			"finalization_started_at",
			"finalized_at",
			"last_heartbeat_at",
			"download_confirmed_at",
			"ended_at",
			"expires_at",
			"last_error_at",
		},
		"session_access_tokens": {"created_at", "last_used_at", "revoked_at"},
		"session_events":        {"created_at"},
		"schema_migrations":     {"applied_at"},
	} {
		for _, column := range columns {
			assertColumnType(t, db, table, column, "TEXT")
		}
	}
	assertAppliedMigrationTimeFormat(t, db)
	for _, index := range []string{
		"idx_sessions_status",
		"idx_sessions_updated_at",
		"idx_sessions_droplet_id",
		"idx_sessions_room_domain",
		"idx_session_access_tokens_session_id",
		"idx_session_events_session_id_id",
	} {
		if !indexExists(t, db, index) {
			t.Fatalf("index %q does not exist", index)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	result, err := Migrate(ctx, db, discardLogger())
	if err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	if result.Current != 1 {
		t.Fatalf("Current = %d", result.Current)
	}
	if len(result.Applied) != 0 {
		t.Fatalf("Applied length = %d", len(result.Applied))
	}
	count, err := AppliedMigrationCount(ctx, db)
	if err != nil {
		t.Fatalf("AppliedMigrationCount() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("AppliedMigrationCount() = %d", count)
	}
}

func TestMigrateRejectsUnknownAppliedMigration(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
insert into schema_migrations(version, name, applied_at)
values (999, 'future', ?);
`, formatSQLiteTime(time.Now())); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}

	_, err := Migrate(ctx, db, discardLogger())
	if err == nil {
		t.Fatal("second Migrate() error = nil, want unknown future migration error")
	}
	if !strings.Contains(err.Error(), "database schema version 999 is newer than this binary supports (1)") {
		t.Fatalf("second Migrate() error = %v", err)
	}
}

func TestTimestampColumnsRejectNonFixedUTCText(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)
	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	valid := formatSQLiteTime(time.Date(2026, 4, 24, 12, 34, 56, 123456789, time.FixedZone("PDT", -7*60*60)))
	insertValidSession(t, db, valid)
	insertValidSessionAccessToken(t, db, valid)
	insertValidSessionEvent(t, db, valid)

	columns := []struct {
		table  string
		column string
		where  string
	}{
		{"sessions", "created_at", "id = 'session_1'"},
		{"sessions", "updated_at", "id = 'session_1'"},
		{"sessions", "ready_at", "id = 'session_1'"},
		{"sessions", "active_at", "id = 'session_1'"},
		{"sessions", "finalization_started_at", "id = 'session_1'"},
		{"sessions", "finalized_at", "id = 'session_1'"},
		{"sessions", "last_heartbeat_at", "id = 'session_1'"},
		{"sessions", "download_confirmed_at", "id = 'session_1'"},
		{"sessions", "ended_at", "id = 'session_1'"},
		{"sessions", "expires_at", "id = 'session_1'"},
		{"sessions", "last_error_at", "id = 'session_1'"},
		{"session_access_tokens", "created_at", "id = 'token_1'"},
		{"session_access_tokens", "last_used_at", "id = 'token_1'"},
		{"session_access_tokens", "revoked_at", "id = 'token_1'"},
		{"session_events", "created_at", "session_id = 'session_1'"},
		{"schema_migrations", "applied_at", "version = 1"},
	}
	invalidValues := []string{
		"not-a-time",
		"2026-04-24T12:34:56.123Z",
		"2026-04-24T12:34:56.123456789-07:00",
		"2026-04-24 12:34:56.123456789Z",
	}

	for _, column := range columns {
		statement := fmt.Sprintf("update %s set %s = ? where %s", column.table, column.column, column.where)
		for _, invalid := range invalidValues {
			if _, err := db.Exec(statement, invalid); err == nil {
				t.Fatalf("update %s.%s to %q succeeded, want check constraint failure", column.table, column.column, invalid)
			}
		}
		if _, err := db.Exec(statement, valid); err != nil {
			t.Fatalf("update %s.%s to valid timestamp: %v", column.table, column.column, err)
		}
	}
}

func openTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return db
}

func insertValidSession(t *testing.T, db *sql.DB, now string) {
	t.Helper()
	if _, err := db.Exec(`
insert into sessions(
  id,
  slug,
  title,
  status,
  machine_token_hash,
  droplet_region,
  droplet_size,
  image_id,
  created_at,
  updated_at
) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`, "session_1", "session-1", "Session 1", "created", "machine_hash", "nyc3", "s-1vcpu-1gb", "image-1", now, now); err != nil {
		t.Fatalf("insert valid session: %v", err)
	}
}

func insertValidSessionAccessToken(t *testing.T, db *sql.DB, now string) {
	t.Helper()
	if _, err := db.Exec(`
insert into session_access_tokens(id, session_id, role, token_hash, created_at)
values (?, ?, ?, ?, ?);
`, "token_1", "session_1", "host", "token_hash", now); err != nil {
		t.Fatalf("insert valid session access token: %v", err)
	}
}

func insertValidSessionEvent(t *testing.T, db *sql.DB, now string) {
	t.Helper()
	if _, err := db.Exec(`
insert into session_events(session_id, type, created_at)
values (?, ?, ?);
`, "session_1", "session.created", now); err != nil {
		t.Fatalf("insert valid session event: %v", err)
	}
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(`select name from sqlite_master where type = 'table' and name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}
	return found == name
}

func indexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(`select name from sqlite_master where type = 'index' and name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query index %q: %v", name, err)
	}
	return found == name
}

func assertStrictTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var strict int
	if err := db.QueryRow(`select strict from pragma_table_list where name = ?`, name).Scan(&strict); err != nil {
		t.Fatalf("query strict table %q: %v", name, err)
	}
	if strict != 1 {
		t.Fatalf("table %q strict = %d, want 1", name, strict)
	}
}

func assertColumnType(t *testing.T, db *sql.DB, table string, column string, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`select type from pragma_table_info(?) where name = ?`, table, column).Scan(&got); err != nil {
		t.Fatalf("query column %q.%q: %v", table, column, err)
	}
	if !strings.EqualFold(got, want) {
		t.Fatalf("column %q.%q type = %q, want %q", table, column, got, want)
	}
}

func assertAppliedMigrationTimeFormat(t *testing.T, db *sql.DB) {
	t.Helper()
	var appliedAt string
	if err := db.QueryRow(`select applied_at from schema_migrations where version = 1`).Scan(&appliedAt); err != nil {
		t.Fatalf("query applied migration time: %v", err)
	}
	if len(appliedAt) != len(formatSQLiteTime(time.Unix(0, 0))) {
		t.Fatalf("applied_at length = %d for %q", len(appliedAt), appliedAt)
	}
	if _, err := time.Parse(sqliteTimeFormat, appliedAt); err != nil {
		t.Fatalf("parse applied_at %q: %v", appliedAt, err)
	}
}

func assertPragma(t *testing.T, db *sql.DB, name string, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`pragma ` + name).Scan(&got); err != nil {
		t.Fatalf("pragma %s: %v", name, err)
	}
	if got != want {
		t.Fatalf("pragma %s = %q, want %q", name, got, want)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %q = %v, want %v", path, got, want)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
