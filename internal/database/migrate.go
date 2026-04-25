package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

const (
	sqliteTimeFormat = "2006-01-02T15:04:05.000000000Z"
	sqliteTimeGlob   = `[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z`
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

type MigrationResult struct {
	Applied []Migration
	Current int
}

var Migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_control_plane_schema",
		SQL: fmt.Sprintf(`
create table sessions (
  id text primary key,
  slug text unique not null,
  title text not null,

  status text not null check (
    status in (
      'created',
      'provisioning',
      'waiting_for_dns',
      'ready',
      'active',
      'finalizing',
      'awaiting_manual_download',
      'teardown_pending',
      'tearing_down',
      'ended',
      'failed'
    )
  ),

  machine_token_hash text,

  droplet_id text,
  droplet_ip text,
  droplet_region text not null,
  droplet_size text not null,
  image_id text not null,

  room_domain text unique,
  dns_record_id text,
  livekit_url text,

  recording_download_url text,
  finalization_summary_json text,

  created_at text not null check (created_at glob '%[1]s'),
  updated_at text not null check (updated_at glob '%[1]s'),
  ready_at text check (ready_at glob '%[1]s'),
  active_at text check (active_at glob '%[1]s'),
  finalization_started_at text check (finalization_started_at glob '%[1]s'),
  finalized_at text check (finalized_at glob '%[1]s'),
  last_heartbeat_at text check (last_heartbeat_at glob '%[1]s'),
  download_confirmed_at text check (download_confirmed_at glob '%[1]s'),
  download_confirmed_by text,
  ended_at text check (ended_at glob '%[1]s'),
  expires_at text check (expires_at glob '%[1]s'),

  last_error text,
  last_error_at text check (last_error_at glob '%[1]s'),
  last_error_phase text,

  provision_attempts integer not null default 0,
  dns_attempts integer not null default 0,
  health_attempts integer not null default 0,
  teardown_attempts integer not null default 0
) strict;

create table session_access_tokens (
  id text primary key,
  session_id text not null,
  role text not null check (role in ('host', 'guest')),
  label text,
  token_hash text not null unique,
  created_at text not null check (created_at glob '%[1]s'),
  last_used_at text check (last_used_at glob '%[1]s'),
  revoked_at text check (revoked_at glob '%[1]s'),

  foreign key (session_id) references sessions(id)
) strict;

create table session_events (
  id integer primary key autoincrement,
  session_id text not null,
  type text not null,
  message text,
  metadata_json text,
  created_at text not null check (created_at glob '%[1]s'),

  foreign key (session_id) references sessions(id)
) strict;

create index idx_sessions_status on sessions(status);
create index idx_sessions_updated_at on sessions(updated_at);
create index idx_sessions_droplet_id on sessions(droplet_id);
create index idx_sessions_room_domain on sessions(room_domain);

create index idx_session_access_tokens_session_id
  on session_access_tokens(session_id);

create index idx_session_events_session_id_id
  on session_events(session_id, id);
`, sqliteTimeGlob),
	},
}

func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) (MigrationResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
create table if not exists schema_migrations (
  version integer primary key,
  name text not null,
  applied_at text not null check (applied_at glob '%[1]s')
) strict;
`, sqliteTimeGlob)); err != nil {
		return MigrationResult{}, fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	appliedVersions, maxAppliedVersion, err := appliedMigrationVersions(ctx, db)
	if err != nil {
		return MigrationResult{}, err
	}
	maxKnownVersion := maxKnownMigrationVersion()
	if maxAppliedVersion > maxKnownVersion {
		return MigrationResult{}, fmt.Errorf("database schema version %d is newer than this binary supports (%d)", maxAppliedVersion, maxKnownVersion)
	}

	result := MigrationResult{}
	for _, migration := range Migrations {
		if appliedVersions[migration.Version] {
			if migration.Version > result.Current {
				result.Current = migration.Version
			}
			continue
		}
		if err := applyMigration(ctx, db, migration); err != nil {
			return MigrationResult{}, err
		}
		logger.InfoContext(ctx, "database migration applied", "version", migration.Version, "name", migration.Name)
		result.Applied = append(result.Applied, migration)
		appliedVersions[migration.Version] = true
		if migration.Version > result.Current {
			result.Current = migration.Version
		}
	}
	return result, nil
}

func appliedMigrationVersions(ctx context.Context, db *sql.DB) (map[int]bool, int, error) {
	rows, err := db.QueryContext(ctx, `select version from schema_migrations`)
	if err != nil {
		return nil, 0, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	versions := make(map[int]bool)
	maxVersion := 0
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, 0, fmt.Errorf("scan applied migration: %w", err)
		}
		versions[version] = true
		if version > maxVersion {
			maxVersion = version
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list applied migrations: %w", err)
	}
	return versions, maxVersion, nil
}

func maxKnownMigrationVersion() int {
	maxVersion := 0
	for _, migration := range Migrations {
		if migration.Version > maxVersion {
			maxVersion = migration.Version
		}
	}
	return maxVersion
}

func applyMigration(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply migration %d %s: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx, `
insert into schema_migrations(version, name, applied_at)
values (?, ?, ?);
`, migration.Version, migration.Name, formatSQLiteTime(time.Now())); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}

func AppliedMigrationCount(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, `select count(*) from schema_migrations`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count applied migrations: %w", err)
	}
	return count, nil
}

func formatSQLiteTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeFormat)
}
