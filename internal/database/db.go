package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	databaseDirMode  os.FileMode = 0o700
	databaseFileMode os.FileMode = 0o600
)

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := configure(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := restrictFilePermissions(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func configure(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pragmas := []string{
		"pragma foreign_keys = on",
		"pragma journal_mode = wal",
		"pragma busy_timeout = 5000",
		"pragma synchronous = normal",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply sqlite %s: %w", pragma, err)
		}
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite database: %w", err)
	}
	return nil
}

func ensureParentDir(path string) error {
	filePath, ok, err := databaseFilePath(path)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, databaseDirMode); err != nil {
		return fmt.Errorf("create database directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, databaseDirMode); err != nil {
		return fmt.Errorf("secure database directory %q: %w", dir, err)
	}
	return nil
}

func restrictFilePermissions(path string) error {
	filePath, ok, err := databaseFilePath(path)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	for _, candidate := range []string{filePath, filePath + "-wal", filePath + "-shm"} {
		if err := os.Chmod(candidate, databaseFileMode); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("secure database file %q: %w", candidate, err)
		}
	}
	return nil
}

func databaseFilePath(path string) (string, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == ":memory:" {
		return "", false, nil
	}
	if !strings.HasPrefix(path, "file:") {
		return path, true, nil
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return "", false, fmt.Errorf("parse sqlite file path %q: %w", path, err)
	}
	if parsed.Query().Get("mode") == "memory" {
		return "", false, nil
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return "", false, nil
	}

	filePath := parsed.Path
	if filePath == "" {
		filePath, err = url.PathUnescape(parsed.Opaque)
		if err != nil {
			return "", false, fmt.Errorf("parse sqlite file path %q: %w", path, err)
		}
	}
	if filePath == "" || filePath == ":memory:" {
		return "", false, nil
	}
	return filePath, true, nil
}
