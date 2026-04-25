package database

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
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
	}
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

func TestMigrateCurrentIgnoresUnknownAppliedMigration(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	if _, err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
insert into schema_migrations(version, name, applied_at)
values (999, 'future', datetime('now'));
`); err != nil {
		t.Fatalf("insert future migration: %v", err)
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
