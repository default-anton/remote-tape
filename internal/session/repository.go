package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const sqliteTimeFormat = "2006-01-02T15:04:05.000000000Z"

var (
	ErrNotFound           = errors.New("session not found")
	ErrInvalidInput       = errors.New("invalid session input")
	ErrSlugConflicts      = errors.New("session slug already exists")
	ErrInvalidToken       = errors.New("invalid session access token")
	ErrMachineTokenLocked = errors.New("machine token is locked after droplet assignment")
)

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

type CreateInput struct {
	Title              string
	Slug               string
	DropletRegion      string
	DropletSize        string
	ImageID            string
	SessionsBaseDomain string
}

type CreateResult struct {
	Session        Session
	HostToken      string
	GuestToken     string
	HostTokenID    string
	GuestTokenID   string
	InitialEventID int64
}

type MachineTokenIssue struct {
	SessionID string
	Token     string
	EventID   int64
}

type Session struct {
	ID                      string  `json:"id"`
	Slug                    string  `json:"slug"`
	Title                   string  `json:"title"`
	Status                  string  `json:"status"`
	DropletID               *string `json:"droplet_id"`
	DropletIP               *string `json:"droplet_ip"`
	DropletRegion           string  `json:"droplet_region"`
	DropletSize             string  `json:"droplet_size"`
	ImageID                 string  `json:"image_id"`
	RoomDomain              *string `json:"room_domain"`
	DNSRecordID             *string `json:"dns_record_id"`
	LiveKitURL              *string `json:"livekit_url"`
	RecordingDownloadURL    *string `json:"recording_download_url"`
	FinalizationSummaryJSON *string `json:"finalization_summary_json"`
	CreatedAt               string  `json:"created_at"`
	UpdatedAt               string  `json:"updated_at"`
	ReadyAt                 *string `json:"ready_at"`
	ActiveAt                *string `json:"active_at"`
	FinalizationStartedAt   *string `json:"finalization_started_at"`
	FinalizedAt             *string `json:"finalized_at"`
	LastHeartbeatAt         *string `json:"last_heartbeat_at"`
	DownloadConfirmedAt     *string `json:"download_confirmed_at"`
	DownloadConfirmedBy     *string `json:"download_confirmed_by"`
	EndedAt                 *string `json:"ended_at"`
	ExpiresAt               *string `json:"expires_at"`
	LastError               *string `json:"last_error"`
	LastErrorAt             *string `json:"last_error_at"`
	LastErrorPhase          *string `json:"last_error_phase"`
	ProvisionAttempts       int64   `json:"provision_attempts"`
	DNSAttempts             int64   `json:"dns_attempts"`
	HealthAttempts          int64   `json:"health_attempts"`
	TeardownAttempts        int64   `json:"teardown_attempts"`
}

type AccessToken struct {
	ID         string  `json:"id"`
	SessionID  string  `json:"session_id"`
	Role       string  `json:"role"`
	Label      *string `json:"label"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
}

type Event struct {
	ID           int64   `json:"id"`
	SessionID    string  `json:"session_id"`
	Type         string  `json:"type"`
	Message      *string `json:"message"`
	MetadataJSON *string `json:"metadata_json"`
	CreatedAt    string  `json:"created_at"`
}

type Detail struct {
	Session      Session       `json:"session"`
	AccessTokens []AccessToken `json:"access_tokens"`
	Events       []Event       `json:"events"`
}

type JoinResult struct {
	Session Session     `json:"session"`
	Token   AccessToken `json:"token"`
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Repository) CreateSession(ctx context.Context, input CreateInput) (CreateResult, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Slug = normalizeSlugInput(input.Slug)
	input.DropletRegion = strings.TrimSpace(input.DropletRegion)
	input.DropletSize = strings.TrimSpace(input.DropletSize)
	input.ImageID = strings.TrimSpace(input.ImageID)
	input.SessionsBaseDomain = strings.ToLower(strings.Trim(strings.TrimSpace(input.SessionsBaseDomain), "."))

	if input.Title == "" {
		return CreateResult{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if len(input.Title) > 100 {
		return CreateResult{}, fmt.Errorf("%w: title must be 100 characters or fewer", ErrInvalidInput)
	}
	if input.Slug == "" {
		input.Slug = Slugify(input.Title)
	}
	if err := validateSlug(input.Slug); err != nil {
		return CreateResult{}, err
	}
	if input.DropletRegion == "" || input.DropletSize == "" || input.ImageID == "" {
		return CreateResult{}, fmt.Errorf("%w: droplet region, droplet size, and image id are required", ErrInvalidInput)
	}

	if exists, err := r.slugExists(ctx, input.Slug); err != nil {
		return CreateResult{}, err
	} else if exists {
		return CreateResult{}, ErrSlugConflicts
	}

	createdAt := formatSQLiteTime(r.now())
	sessionID, err := randomID("sess")
	if err != nil {
		return CreateResult{}, err
	}
	hostToken, err := randomToken(32)
	if err != nil {
		return CreateResult{}, err
	}
	guestToken, err := randomToken(32)
	if err != nil {
		return CreateResult{}, err
	}
	hostTokenID, err := randomID("sat")
	if err != nil {
		return CreateResult{}, err
	}
	guestTokenID, err := randomID("sat")
	if err != nil {
		return CreateResult{}, err
	}

	roomDomain, err := newRoomDomain(input.SessionsBaseDomain)
	if err != nil {
		return CreateResult{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateResult{}, fmt.Errorf("begin create session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
insert into sessions(
  id, slug, title, status, machine_token_hash,
  droplet_region, droplet_size, image_id, room_domain,
  created_at, updated_at
) values (?, ?, ?, 'created', ?, ?, ?, ?, ?, ?, ?);
`, sessionID, input.Slug, input.Title, nil, input.DropletRegion, input.DropletSize, input.ImageID, roomDomain, createdAt, createdAt); err != nil {
		if strings.Contains(err.Error(), "constraint failed") {
			return CreateResult{}, ErrSlugConflicts
		}
		return CreateResult{}, fmt.Errorf("insert session: %w", err)
	}

	for _, token := range []struct {
		id    string
		role  string
		raw   string
		label string
	}{
		{hostTokenID, "host", hostToken, "Initial host"},
		{guestTokenID, "guest", guestToken, "Initial guest"},
	} {
		if _, err := tx.ExecContext(ctx, `
insert into session_access_tokens(id, session_id, role, label, token_hash, created_at)
values (?, ?, ?, ?, ?, ?);
`, token.id, sessionID, token.role, token.label, HashToken(token.raw), createdAt); err != nil {
			return CreateResult{}, fmt.Errorf("insert %s access token: %w", token.role, err)
		}
	}

	eventID, err := appendEvent(ctx, tx, sessionID, "session.created", "Session created", nil, createdAt)
	if err != nil {
		return CreateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreateResult{}, fmt.Errorf("commit create session: %w", err)
	}

	detail, err := r.GetSession(ctx, sessionID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{
		Session:        detail.Session,
		HostToken:      hostToken,
		GuestToken:     guestToken,
		HostTokenID:    hostTokenID,
		GuestTokenID:   guestTokenID,
		InitialEventID: eventID,
	}, nil
}

func (r *Repository) ListSessions(ctx context.Context) ([]Session, error) {
	rows, err := r.db.QueryContext(ctx, `
select `+sessionColumns+`
from sessions
order by updated_at desc, created_at desc, id desc;
`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

func (r *Repository) ListProvisioningCandidates(ctx context.Context, limit int) ([]Session, error) {
	return r.listSessionsByStatus(ctx, "created", limit, "provisioning candidate")
}

func (r *Repository) ListProvisioningSessions(ctx context.Context, limit int) ([]Session, error) {
	return r.listSessionsByStatus(ctx, "provisioning", limit, "provisioning session")
}

func (r *Repository) ListTearingDownSessions(ctx context.Context, limit int) ([]Session, error) {
	return r.listSessionsByStatus(ctx, "tearing_down", limit, "tearing down session")
}

func (r *Repository) MarkProvisioningStarted(ctx context.Context, sessionID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrNotFound
	}
	now := formatSQLiteTime(r.now())
	return r.execSessionTransition(ctx, sessionID, sessionTransition{
		operation: "mark provisioning started",
		query: `
update sessions
set status = 'provisioning',
    provision_attempts = provision_attempts + 1,
    last_error = null,
    last_error_at = null,
    last_error_phase = null,
    updated_at = ?
where id = ? and status = 'created';
`,
		args:         []any{now, sessionID},
		eventType:    "provisioning.started",
		eventMessage: "Provisioning started",
		at:           now,
	})
}

type DropletAssignmentResult struct {
	Accepted bool
	Changed  bool
	Status   string
}

func (r *Repository) AssignDroplet(ctx context.Context, sessionID string, dropletID string, dropletIP string, adopted bool) (DropletAssignmentResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	dropletID = strings.TrimSpace(dropletID)
	dropletIP = strings.TrimSpace(dropletIP)
	if sessionID == "" || dropletID == "" {
		return DropletAssignmentResult{}, ErrNotFound
	}
	now := formatSQLiteTime(r.now())
	nextStatus := "provisioning"
	if dropletIP != "" {
		nextStatus = "waiting_for_dns"
	}
	eventType := "provisioning.droplet_created"
	message := "Session droplet created"
	if adopted {
		eventType = "provisioning.droplet_adopted"
		message = "Existing session droplet adopted"
	}
	if dropletIP == "" {
		eventType = "provisioning.waiting_for_ip"
		message = "Session droplet assigned; waiting for public IPv4"
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return DropletAssignmentResult{}, fmt.Errorf("begin assign droplet: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus string
	var existingDropletID, existingDropletIP sql.NullString
	err = tx.QueryRowContext(ctx, `select status, droplet_id, droplet_ip from sessions where id = ?;`, sessionID).Scan(&currentStatus, &existingDropletID, &existingDropletIP)
	if errors.Is(err, sql.ErrNoRows) {
		return DropletAssignmentResult{}, nil
	}
	if err != nil {
		return DropletAssignmentResult{}, fmt.Errorf("read session before assign droplet: %w", err)
	}
	if currentStatus == "tearing_down" {
		if existingDropletID.Valid && existingDropletID.String == dropletID && nullStringValue(existingDropletIP) == dropletIP {
			return DropletAssignmentResult{Accepted: true, Status: currentStatus}, nil
		}
		result, err := tx.ExecContext(ctx, `
update sessions
set droplet_id = ?,
    droplet_ip = nullif(?, ''),
    updated_at = ?
where id = ? and status = 'tearing_down';
`, dropletID, dropletIP, now, sessionID)
		if err != nil {
			return DropletAssignmentResult{}, fmt.Errorf("assign droplet during teardown: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return DropletAssignmentResult{}, fmt.Errorf("read teardown droplet assignment rows affected: %w", err)
		}
		if changed == 0 {
			return DropletAssignmentResult{}, nil
		}
		if _, err := appendEvent(ctx, tx, sessionID, "teardown.droplet_discovered", "Session droplet discovered after force destroy request", nil, now); err != nil {
			return DropletAssignmentResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return DropletAssignmentResult{}, fmt.Errorf("commit teardown droplet assignment: %w", err)
		}
		return DropletAssignmentResult{Accepted: true, Changed: true, Status: currentStatus}, nil
	}
	if currentStatus != "provisioning" {
		return DropletAssignmentResult{Status: currentStatus}, nil
	}
	if dropletIP == "" && existingDropletID.Valid && existingDropletID.String == dropletID && !existingDropletIP.Valid {
		return DropletAssignmentResult{Accepted: true, Status: currentStatus}, nil
	}

	result, err := tx.ExecContext(ctx, `
update sessions
set status = ?,
    droplet_id = ?,
    droplet_ip = nullif(?, ''),
    last_error = null,
    last_error_at = null,
    last_error_phase = null,
    updated_at = ?
where id = ? and status = 'provisioning';
`, nextStatus, dropletID, dropletIP, now, sessionID)
	if err != nil {
		return DropletAssignmentResult{}, fmt.Errorf("assign droplet: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return DropletAssignmentResult{}, fmt.Errorf("read assign droplet rows affected: %w", err)
	}
	if changed == 0 {
		return DropletAssignmentResult{}, nil
	}
	if _, err := appendEvent(ctx, tx, sessionID, eventType, message, nil, now); err != nil {
		return DropletAssignmentResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DropletAssignmentResult{}, fmt.Errorf("commit assign droplet: %w", err)
	}
	return DropletAssignmentResult{Accepted: true, Changed: true, Status: nextStatus}, nil
}

func (r *Repository) MarkForceDestroyStarted(ctx context.Context, sessionID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrNotFound
	}
	now := formatSQLiteTime(r.now())
	return r.execSessionTransition(ctx, sessionID, sessionTransition{
		operation: "mark force destroy started",
		query: `
update sessions
set status = 'tearing_down',
    last_error = null,
    last_error_at = null,
    last_error_phase = null,
    updated_at = ?
where id = ? and status in ('provisioning', 'waiting_for_dns', 'failed');
`,
		args:         []any{now, sessionID},
		eventType:    "session.force_destroy_started",
		eventMessage: "Force destroy session server started",
		at:           now,
	})
}

func (r *Repository) MarkForceDestroyed(ctx context.Context, sessionID string, dropletID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrNotFound
	}
	dropletID = strings.TrimSpace(dropletID)
	now := formatSQLiteTime(r.now())
	message := "Session server force destroyed"
	if dropletID != "" {
		message += " (droplet " + dropletID + ")"
	}
	return r.execSessionTransition(ctx, sessionID, sessionTransition{
		operation: "mark force destroyed",
		query: `
update sessions
set status = 'ended',
    ended_at = ?,
    last_error = null,
    last_error_at = null,
    last_error_phase = null,
    updated_at = ?
where id = ? and status = 'tearing_down';
`,
		args:         []any{now, now, sessionID},
		eventType:    "session.force_destroyed",
		eventMessage: message,
		at:           now,
	})
}

func (r *Repository) MarkForceDestroyFailed(ctx context.Context, sessionID string, cause error) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrNotFound
	}
	now := formatSQLiteTime(r.now())
	message := capErrorMessage(cause)
	return r.execSessionTransition(ctx, sessionID, sessionTransition{
		operation: "mark force destroy failed",
		query: `
update sessions
set status = 'failed',
    last_error = ?,
    last_error_at = ?,
    last_error_phase = 'teardown',
    updated_at = ?
where id = ? and status = 'tearing_down';
`,
		args:         []any{message, now, now, sessionID},
		eventType:    "session.force_destroy_failed",
		eventMessage: "Force destroy session server failed",
		at:           now,
	})
}

func (r *Repository) MarkProvisioningFailed(ctx context.Context, sessionID string, cause error) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, ErrNotFound
	}
	now := formatSQLiteTime(r.now())
	message := capErrorMessage(cause)
	return r.execSessionTransition(ctx, sessionID, sessionTransition{
		operation: "mark provisioning failed",
		query: `
update sessions
set status = 'failed',
    last_error = ?,
    last_error_at = ?,
    last_error_phase = 'provisioning',
    updated_at = ?
where id = ? and status = 'provisioning';
`,
		args:         []any{message, now, now, sessionID},
		eventType:    "provisioning.failed",
		eventMessage: "Provisioning failed",
		at:           now,
	})
}

func (r *Repository) GetSession(ctx context.Context, id string) (Detail, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Detail{}, ErrNotFound
	}
	row := r.db.QueryRowContext(ctx, `select `+sessionColumns+` from sessions where id = ?;`, id)
	s, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	tokens, err := r.listAccessTokens(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	events, err := r.listEvents(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Session: s, AccessTokens: tokens, Events: events}, nil
}

func (r *Repository) JoinSession(ctx context.Context, slug string, rawToken string) (JoinResult, error) {
	slug = normalizeSlugInput(slug)
	rawToken = strings.TrimSpace(rawToken)
	if slug == "" || rawToken == "" {
		return JoinResult{}, ErrInvalidToken
	}
	now := formatSQLiteTime(r.now())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return JoinResult{}, fmt.Errorf("begin join session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
select `+prefixedSessionColumns("s")+`, t.id, t.session_id, t.role, t.label, t.created_at, t.last_used_at, t.revoked_at
from sessions s
join session_access_tokens t on t.session_id = s.id
where s.slug = ? and t.token_hash = ? and t.revoked_at is null;
`, slug, HashToken(rawToken))
	s, token, err := scanSessionAndToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return JoinResult{}, ErrInvalidToken
	}
	if err != nil {
		return JoinResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
update session_access_tokens set last_used_at = ? where id = ?;
`, now, token.ID); err != nil {
		return JoinResult{}, fmt.Errorf("record access token use: %w", err)
	}
	token.LastUsedAt = &now
	if err := tx.Commit(); err != nil {
		return JoinResult{}, fmt.Errorf("commit join session: %w", err)
	}
	return JoinResult{Session: s, Token: token}, nil
}

// IssueMachineToken returns plaintext once and refuses to rotate after droplet assignment so existing callback auth stays valid.
func (r *Repository) IssueMachineToken(ctx context.Context, sessionID string) (MachineTokenIssue, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return MachineTokenIssue{}, ErrNotFound
	}
	now := formatSQLiteTime(r.now())

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MachineTokenIssue{}, fmt.Errorf("begin issue machine token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingHash sql.NullString
	var dropletID sql.NullString
	if err := tx.QueryRowContext(ctx, `
select machine_token_hash, droplet_id from sessions where id = ?;
`, sessionID).Scan(&existingHash, &dropletID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return MachineTokenIssue{}, ErrNotFound
		}
		return MachineTokenIssue{}, fmt.Errorf("load session for machine token issue: %w", err)
	}
	if dropletID.Valid {
		return MachineTokenIssue{}, ErrMachineTokenLocked
	}

	token, err := randomToken(32)
	if err != nil {
		return MachineTokenIssue{}, err
	}
	eventType := "session.machine_token_issued"
	message := "Machine token issued for provisioning"
	if existingHash.Valid {
		eventType = "session.machine_token_rotated"
		message = "Machine token rotated before droplet assignment"
	}

	if _, err := tx.ExecContext(ctx, `
update sessions set machine_token_hash = ?, updated_at = ? where id = ?;
`, HashToken(token), now, sessionID); err != nil {
		return MachineTokenIssue{}, fmt.Errorf("store machine token hash: %w", err)
	}
	eventID, err := appendEvent(ctx, tx, sessionID, eventType, message, nil, now)
	if err != nil {
		return MachineTokenIssue{}, err
	}
	if err := tx.Commit(); err != nil {
		return MachineTokenIssue{}, fmt.Errorf("commit machine token issue: %w", err)
	}
	return MachineTokenIssue{SessionID: sessionID, Token: token, EventID: eventID}, nil
}

func (r *Repository) listSessionsByStatus(ctx context.Context, status string, limit int, label string) ([]Session, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: %s limit must be positive", ErrInvalidInput, label)
	}
	rows, err := r.db.QueryContext(ctx, `
select `+sessionColumns+`
from sessions
where status = ?
order by updated_at asc, id asc
limit ?;
`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list %ss: %w", label, err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %ss: %w", label, err)
	}
	return sessions, nil
}

func (r *Repository) slugExists(ctx context.Context, slug string) (bool, error) {
	var found string
	err := r.db.QueryRowContext(ctx, `select slug from sessions where slug = ?;`, slug).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check slug availability: %w", err)
	}
	return true, nil
}

func (r *Repository) listAccessTokens(ctx context.Context, sessionID string) ([]AccessToken, error) {
	rows, err := r.db.QueryContext(ctx, `
select id, session_id, role, label, created_at, last_used_at, revoked_at
from session_access_tokens
where session_id = ?
order by created_at, id;
`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list access tokens: %w", err)
	}
	defer rows.Close()
	var tokens []AccessToken
	for rows.Next() {
		t, err := scanAccessToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list access tokens: %w", err)
	}
	return tokens, nil
}

func (r *Repository) listEvents(ctx context.Context, sessionID string) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, `
select id, session_id, type, message, metadata_json, created_at
from session_events
where session_id = ?
order by id asc;
`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	return events, nil
}

func AppendEvent(ctx context.Context, db *sql.DB, sessionID string, eventType string, message *string, metadataJSON *string) (int64, error) {
	return appendEvent(ctx, db, sessionID, eventType, messageValue(message), metadataJSON, formatSQLiteTime(time.Now()))
}

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type sessionTransition struct {
	operation    string
	query        string
	args         []any
	eventType    string
	eventMessage string
	at           string
}

func (r *Repository) execSessionTransition(ctx context.Context, sessionID string, transition sessionTransition) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin %s: %w", transition.operation, err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, transition.query, transition.args...)
	if err != nil {
		return false, fmt.Errorf("%s: %w", transition.operation, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read %s rows affected: %w", transition.operation, err)
	}
	if changed == 0 {
		return false, nil
	}
	if _, err := appendEvent(ctx, tx, sessionID, transition.eventType, transition.eventMessage, nil, transition.at); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit %s: %w", transition.operation, err)
	}
	return true, nil
}

func appendEvent(ctx context.Context, q queryer, sessionID string, eventType string, message string, metadataJSON *string, createdAt string) (int64, error) {
	var messageArg any
	if strings.TrimSpace(message) != "" {
		messageArg = message
	}
	result, err := q.ExecContext(ctx, `
insert into session_events(session_id, type, message, metadata_json, created_at)
values (?, ?, ?, ?, ?);
`, sessionID, eventType, messageArg, metadataJSON, createdAt)
	if err != nil {
		return 0, fmt.Errorf("append session event %q: %w", eventType, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read session event id: %w", err)
	}
	return id, nil
}

func messageValue(message *string) string {
	if message == nil {
		return ""
	}
	return *message
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func capErrorMessage(cause error) string {
	message := "provisioning failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = strings.TrimSpace(cause.Error())
	}
	runes := []rune(message)
	if len(runes) > 2000 {
		return string(runes[:2000])
	}
	return message
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 63 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "session"
	}
	return slug
}

func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	if len(slug) > 63 {
		return fmt.Errorf("%w: slug must be 63 characters or fewer", ErrInvalidInput)
	}
	if strings.HasPrefix(slug, "-") || strings.HasSuffix(slug, "-") {
		return fmt.Errorf("%w: slug must not start or end with a dash", ErrInvalidInput)
	}
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Errorf("%w: slug may contain only lowercase letters, numbers, and dashes", ErrInvalidInput)
	}
	return nil
}

const (
	maxDNSLabelLength = 63
	maxDNSNameLength  = 253
	roomLabelPrefix   = "room-"
)

func newRoomDomain(baseDomain string) (*string, error) {
	baseDomain = strings.ToLower(strings.Trim(strings.TrimSpace(baseDomain), "."))
	if baseDomain == "" {
		return nil, nil
	}
	if err := validateDNSName(baseDomain); err != nil {
		return nil, fmt.Errorf("%w: sessions base domain %q is not a valid DNS name: %v", ErrInvalidInput, baseDomain, err)
	}
	label, err := randomRoomLabel()
	if err != nil {
		return nil, err
	}
	domain := label + "." + baseDomain
	if len(domain) > maxDNSNameLength {
		return nil, fmt.Errorf("%w: room domain %q is longer than %d characters", ErrInvalidInput, domain, maxDNSNameLength)
	}
	return &domain, nil
}

func randomRoomLabel() (string, error) {
	value, err := randomBase32(16)
	if err != nil {
		return "", err
	}
	label := roomLabelPrefix + value
	if len(label) > maxDNSLabelLength {
		return "", fmt.Errorf("generated room label %q is longer than %d characters", label, maxDNSLabelLength)
	}
	return label, nil
}

func validateDNSName(name string) error {
	if name == "" || len(name) > maxDNSNameLength {
		return errors.New("invalid DNS name length")
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > maxDNSLabelLength {
			return errors.New("invalid DNS label length")
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return errors.New("DNS labels must not start or end with a dash")
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return errors.New("DNS labels may contain only lowercase letters, numbers, and dashes")
		}
	}
	return nil
}

func normalizeSlugInput(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

func randomID(prefix string) (string, error) {
	value, err := randomBase32(16)
	if err != nil {
		return "", err
	}
	return prefix + "_" + value, nil
}

func randomBase32(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random value: %w", err)
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func formatSQLiteTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeFormat)
}
