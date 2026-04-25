package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
		SQL: `
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

  machine_token_hash text not null,

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

  created_at datetime not null,
  updated_at datetime not null,
  ready_at datetime,
  active_at datetime,
  finalization_started_at datetime,
  finalized_at datetime,
  last_heartbeat_at datetime,
  download_confirmed_at datetime,
  download_confirmed_by text,
  ended_at datetime,
  expires_at datetime,

  last_error text,
  last_error_at datetime,
  last_error_phase text,

  provision_attempts integer not null default 0,
  dns_attempts integer not null default 0,
  health_attempts integer not null default 0,
  teardown_attempts integer not null default 0
);

create table session_access_tokens (
  id text primary key,
  session_id text not null,
  role text not null check (role in ('host', 'guest')),
  label text,
  token_hash text not null unique,
  created_at datetime not null,
  last_used_at datetime,
  revoked_at datetime,

  foreign key (session_id) references sessions(id)
);

create table session_events (
  id integer primary key autoincrement,
  session_id text not null,
  type text not null,
  message text,
  metadata_json text,
  created_at datetime not null,

  foreign key (session_id) references sessions(id)
);

create index idx_sessions_status on sessions(status);
create index idx_sessions_updated_at on sessions(updated_at);
create index idx_sessions_droplet_id on sessions(droplet_id);
create index idx_sessions_room_domain on sessions(room_domain);

create index idx_session_access_tokens_session_id
  on session_access_tokens(session_id);

create index idx_session_events_session_id_id
  on session_events(session_id, id);
`,
	},
}

func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) (MigrationResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if _, err := db.ExecContext(ctx, `
create table if not exists schema_migrations (
  version integer primary key,
  name text not null,
  applied_at datetime not null
);
`); err != nil {
		return MigrationResult{}, fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	result := MigrationResult{}
	for _, migration := range Migrations {
		applied, err := migrationApplied(ctx, db, migration.Version)
		if err != nil {
			return MigrationResult{}, err
		}
		if applied {
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
		if migration.Version > result.Current {
			result.Current = migration.Version
		}
	}
	return result, nil
}

func migrationApplied(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `select 1 from schema_migrations where version = ?`, version).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("check migration %d: %w", version, err)
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
values (?, ?, datetime('now'));
`, migration.Version, migration.Name); err != nil {
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
