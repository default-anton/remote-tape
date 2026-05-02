package session

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/default-anton/remote-tape/internal/database"
)

func TestCreateSessionPersistsSessionTokensAndEvent(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 123, time.UTC) }

	created, err := repo.CreateSession(ctx, CreateInput{
		Title:              "The Infra Podcast #313",
		Slug:               "the-infra-podcast-313",
		DropletRegion:      "nyc3",
		DropletSize:        "s-2vcpu-2gb",
		ImageID:            "ubuntu-24-04-x64",
		SessionsBaseDomain: "sessions.example.com",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Session.ID == "" || created.Session.Status != "created" {
		t.Fatalf("created session = %+v", created.Session)
	}
	assertRoomDomain(t, created.Session.RoomDomain, "sessions.example.com")
	if created.HostToken == "" || created.GuestToken == "" || created.HostToken == created.GuestToken {
		t.Fatalf("host/guest tokens not unique: host=%q guest=%q", created.HostToken, created.GuestToken)
	}

	var machineTokenHash sql.NullString
	if err := db.QueryRowContext(ctx, `select machine_token_hash from sessions where id = ?`, created.Session.ID).Scan(&machineTokenHash); err != nil {
		t.Fatalf("query machine_token_hash: %v", err)
	}
	if machineTokenHash.Valid {
		t.Fatalf("machine_token_hash = %q, want null until provisioning", machineTokenHash.String)
	}

	rows, err := db.QueryContext(ctx, `select role, token_hash from session_access_tokens where session_id = ? order by role`, created.Session.ID)
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	defer rows.Close()
	seen := map[string]string{}
	for rows.Next() {
		var role, hash string
		if err := rows.Scan(&role, &hash); err != nil {
			t.Fatalf("scan token: %v", err)
		}
		seen[role] = hash
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tokens: %v", err)
	}
	if seen["host"] != HashToken(created.HostToken) {
		t.Fatalf("host token hash = %q", seen["host"])
	}
	if seen["guest"] != HashToken(created.GuestToken) {
		t.Fatalf("guest token hash = %q", seen["guest"])
	}

	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.AccessTokens) != 2 {
		t.Fatalf("AccessTokens length = %d", len(detail.AccessTokens))
	}
	if len(detail.Events) != 1 || detail.Events[0].Type != "session.created" {
		t.Fatalf("Events = %+v", detail.Events)
	}
}

func TestListProvisioningCandidatesReturnsCreatedOrderedAndBounded(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	for _, item := range []struct {
		id        string
		title     string
		slug      string
		updatedAt string
		status    string
	}{
		{"sess_newest", "Newest Created", "newest-created", "2026-04-24T12:03:00.000000000Z", "created"},
		{"sess_b", "Old Created B", "old-created-b", "2026-04-24T12:00:00.000000000Z", "created"},
		{"sess_a", "Old Created A", "old-created-a", "2026-04-24T12:00:00.000000000Z", "created"},
		{"sess_provisioning", "Already Provisioning", "already-provisioning", "2026-04-24T11:00:00.000000000Z", "provisioning"},
	} {
		if _, err := db.ExecContext(ctx, `
insert into sessions(id, slug, title, status, droplet_region, droplet_size, image_id, created_at, updated_at)
values (?, ?, ?, ?, 'nyc3', 's-1vcpu-1gb', 'image', ?, ?);
`, item.id, item.slug, item.title, item.status, item.updatedAt, item.updatedAt); err != nil {
			t.Fatalf("insert session %q: %v", item.id, err)
		}
	}

	candidates, err := repo.ListProvisioningCandidates(ctx, 2)
	if err != nil {
		t.Fatalf("ListProvisioningCandidates() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d", len(candidates))
	}
	if candidates[0].Slug != "old-created-a" || candidates[1].Slug != "old-created-b" {
		t.Fatalf("candidate order = %q, %q", candidates[0].Slug, candidates[1].Slug)
	}
}

func TestMarkProvisioningStartedTransitionsCreatedAndAppendsEvent(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Start Provisioning", Slug: "start-provisioning", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set last_error = 'old', last_error_at = created_at, last_error_phase = 'old' where id = ?`, created.Session.ID); err != nil {
		t.Fatalf("seed error fields: %v", err)
	}
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC) }

	changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("MarkProvisioningStarted() error = %v", err)
	}
	if !changed {
		t.Fatal("MarkProvisioningStarted() changed = false")
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "provisioning" || detail.Session.ProvisionAttempts != 1 {
		t.Fatalf("session after transition = %+v", detail.Session)
	}
	if detail.Session.LastError != nil || detail.Session.LastErrorAt != nil || detail.Session.LastErrorPhase != nil {
		t.Fatalf("error fields not cleared: %+v", detail.Session)
	}
	if detail.Session.UpdatedAt != "2026-04-24T15:00:00.000000000Z" {
		t.Fatalf("updated_at = %q", detail.Session.UpdatedAt)
	}
	if len(detail.Events) != 2 || detail.Events[1].Type != "provisioning.started" || *detail.Events[1].Message != "Provisioning started" {
		t.Fatalf("events = %+v", detail.Events)
	}
}

func TestMarkProvisioningStartedNoOpsWhenNotCreated(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Noop Started", Slug: "noop-started", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("first MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("second MarkProvisioningStarted() error = %v", err)
	}
	if changed {
		t.Fatal("second MarkProvisioningStarted() changed = true")
	}
	detail, _ := repo.GetSession(ctx, created.Session.ID)
	if detail.Session.ProvisionAttempts != 1 || len(detail.Events) != 2 {
		t.Fatalf("duplicate transition persisted: %+v events=%+v", detail.Session, detail.Events)
	}
}

func TestMarkProvisioningFailedTransitionsProvisioningAndCapsError(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Fail Provisioning", Slug: "fail-provisioning", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 16, 0, 0, 0, time.UTC) }

	changed, err := repo.MarkProvisioningFailed(ctx, created.Session.ID, errors.New(strings.Repeat("x", 2100)))
	if err != nil {
		t.Fatalf("MarkProvisioningFailed() error = %v", err)
	}
	if !changed {
		t.Fatal("MarkProvisioningFailed() changed = false")
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "failed" || detail.Session.ProvisionAttempts != 1 {
		t.Fatalf("session after failure = %+v", detail.Session)
	}
	if detail.Session.LastError == nil || len(*detail.Session.LastError) != 2000 {
		t.Fatalf("last_error length = %v", detail.Session.LastError)
	}
	if detail.Session.LastErrorAt == nil || *detail.Session.LastErrorAt != "2026-04-24T16:00:00.000000000Z" {
		t.Fatalf("last_error_at = %v", detail.Session.LastErrorAt)
	}
	if detail.Session.LastErrorPhase == nil || *detail.Session.LastErrorPhase != "provisioning" {
		t.Fatalf("last_error_phase = %v", detail.Session.LastErrorPhase)
	}
	if len(detail.Events) != 3 || detail.Events[2].Type != "provisioning.failed" || *detail.Events[2].Message != "Provisioning failed" {
		t.Fatalf("events = %+v", detail.Events)
	}
}

func TestMarkProvisioningFailedNoOpsWhenNotProvisioning(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Noop Failed", Slug: "noop-failed", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	changed, err := repo.MarkProvisioningFailed(ctx, created.Session.ID, errors.New("boom"))
	if err != nil {
		t.Fatalf("MarkProvisioningFailed() error = %v", err)
	}
	if changed {
		t.Fatal("MarkProvisioningFailed() changed = true")
	}
	detail, _ := repo.GetSession(ctx, created.Session.ID)
	if detail.Session.Status != "created" || detail.Session.LastError != nil || len(detail.Events) != 1 {
		t.Fatalf("unexpected failure transition: %+v events=%+v", detail.Session, detail.Events)
	}
}

func TestAssignDropletTransitionsAndWaitingForIP(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Assign Droplet", Slug: "assign-droplet", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}

	assignment, err := repo.AssignDroplet(ctx, created.Session.ID, "123", "", false)
	if err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignDroplet(no ip) assignment=%+v error=%v", assignment, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "provisioning" || detail.Session.DropletID == nil || *detail.Session.DropletID != "123" || detail.Session.DropletIP != nil {
		t.Fatalf("session after no-ip assign = %+v", detail.Session)
	}
	if detail.Events[len(detail.Events)-1].Type != "provisioning.waiting_for_ip" {
		t.Fatalf("last event = %+v", detail.Events[len(detail.Events)-1])
	}

	assignment, err = repo.AssignDroplet(ctx, created.Session.ID, "123", "", false)
	if err != nil {
		t.Fatalf("AssignDroplet(repeated no ip) error = %v", err)
	}
	if assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignDroplet repeated no-ip assignment = %+v", assignment)
	}
	detail, err = repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.Events) != 3 {
		t.Fatalf("repeated no-ip assignment appended events = %+v", detail.Events)
	}

	assignment, err = repo.AssignDroplet(ctx, created.Session.ID, "123", "203.0.113.10", true)
	if err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignDroplet(with ip) assignment=%+v error=%v", assignment, err)
	}
	detail, err = repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "waiting_for_dns" || detail.Session.DropletIP == nil || *detail.Session.DropletIP != "203.0.113.10" {
		t.Fatalf("session after ip assign = %+v", detail.Session)
	}
	if detail.Events[len(detail.Events)-1].Type != "provisioning.droplet_adopted" {
		t.Fatalf("last event = %+v", detail.Events[len(detail.Events)-1])
	}

	assignment, err = repo.AssignDroplet(ctx, created.Session.ID, "123", "203.0.113.10", true)
	if err != nil {
		t.Fatalf("AssignDroplet(noop) error = %v", err)
	}
	if assignment.Changed || assignment.Accepted {
		t.Fatalf("AssignDroplet outside provisioning assignment = %+v", assignment)
	}
}

func TestForceDestroyLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Force Destroy", Slug: "force-destroy", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	if assignment, err := repo.AssignDroplet(ctx, created.Session.ID, "123", "203.0.113.10", false); err != nil || !assignment.Changed {
		t.Fatalf("AssignDroplet() assignment=%+v error=%v", assignment, err)
	}

	changed, err := repo.MarkForceDestroyStarted(ctx, created.Session.ID)
	if err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "tearing_down" || detail.Events[len(detail.Events)-1].Type != "session.force_destroy_started" {
		t.Fatalf("after force destroy start session=%+v events=%+v", detail.Session, detail.Events)
	}
	if assignment, err := repo.AssignDroplet(ctx, created.Session.ID, "456", "203.0.113.11", false); err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignDroplet while tearing_down assignment=%+v error=%v", assignment, err)
	}
	if changed, err := repo.MarkForceDestroyed(ctx, created.Session.ID, "456"); err != nil || !changed {
		t.Fatalf("MarkForceDestroyed() changed=%v error=%v", changed, err)
	}
	detail, err = repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "ended" || detail.Session.EndedAt == nil || detail.Events[len(detail.Events)-1].Type != "session.force_destroyed" {
		t.Fatalf("after force destroyed session=%+v events=%+v", detail.Session, detail.Events)
	}
}

func TestForceDestroyFailureReturnsToFailedForRetry(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Force Destroy Failure", Slug: "force-destroy-failure", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set status = 'failed' where id = ?;`, created.Session.ID); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if changed, err := repo.MarkForceDestroyStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	if changed, err := repo.MarkForceDestroyFailed(ctx, created.Session.ID, errors.New("digitalocean unavailable")); err != nil || !changed {
		t.Fatalf("MarkForceDestroyFailed() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "failed" || detail.Session.LastError == nil || *detail.Session.LastError != "digitalocean unavailable" || detail.Session.LastErrorPhase == nil || *detail.Session.LastErrorPhase != "teardown" {
		t.Fatalf("after force destroy failed session=%+v", detail.Session)
	}
	if detail.Events[len(detail.Events)-1].Type != "session.force_destroy_failed" {
		t.Fatalf("last event = %+v", detail.Events[len(detail.Events)-1])
	}
}

func TestIssueMachineTokenStoresHashAndEvent(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Needs Machine Token", Slug: "needs-machine-token", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC) }

	issued, err := repo.IssueMachineToken(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("IssueMachineToken() error = %v", err)
	}
	if issued.SessionID != created.Session.ID || issued.Token == "" || issued.EventID == 0 {
		t.Fatalf("IssueMachineToken() = %+v", issued)
	}

	var machineTokenHash, updatedAt string
	if err := db.QueryRowContext(ctx, `select machine_token_hash, updated_at from sessions where id = ?`, created.Session.ID).Scan(&machineTokenHash, &updatedAt); err != nil {
		t.Fatalf("query machine token hash: %v", err)
	}
	if machineTokenHash != HashToken(issued.Token) {
		t.Fatalf("machine_token_hash = %q", machineTokenHash)
	}
	if updatedAt != "2026-04-24T14:00:00.000000000Z" {
		t.Fatalf("updated_at = %q", updatedAt)
	}

	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.Events) != 2 || detail.Events[1].Type != "session.machine_token_issued" {
		t.Fatalf("Events = %+v", detail.Events)
	}
}

func TestIssueMachineTokenRotatesBeforeDropletAssignment(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Rotate Machine Token", Slug: "rotate-machine-token", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	first, err := repo.IssueMachineToken(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("first IssueMachineToken() error = %v", err)
	}
	second, err := repo.IssueMachineToken(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("second IssueMachineToken() error = %v", err)
	}
	if first.Token == second.Token {
		t.Fatalf("machine token did not rotate: %q", first.Token)
	}

	var machineTokenHash string
	if err := db.QueryRowContext(ctx, `select machine_token_hash from sessions where id = ?`, created.Session.ID).Scan(&machineTokenHash); err != nil {
		t.Fatalf("query machine token hash: %v", err)
	}
	if machineTokenHash != HashToken(second.Token) {
		t.Fatalf("machine_token_hash = %q", machineTokenHash)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.Events) != 3 || detail.Events[2].Type != "session.machine_token_rotated" {
		t.Fatalf("Events = %+v", detail.Events)
	}
}

func TestIssueMachineTokenRejectsAfterDropletAssignment(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Locked Machine Token", Slug: "locked-machine-token", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set droplet_id = 'do-123' where id = ?`, created.Session.ID); err != nil {
		t.Fatalf("set droplet id: %v", err)
	}

	_, err = repo.IssueMachineToken(ctx, created.Session.ID)
	if !errors.Is(err, ErrMachineTokenLocked) {
		t.Fatalf("IssueMachineToken() error = %v, want ErrMachineTokenLocked", err)
	}
}

func TestCreateSessionUsesDNSSafeRoomDomainForMaxLengthSlug(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	slug := strings.Repeat("a", 63)

	created, err := repo.CreateSession(ctx, CreateInput{Title: "Max Slug", Slug: slug, DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	assertRoomDomain(t, created.Session.RoomDomain, "sessions.example.com")
	if strings.Contains(*created.Session.RoomDomain, slug) {
		t.Fatalf("RoomDomain = %q includes full user slug and risks invalid DNS labels", *created.Session.RoomDomain)
	}
}

func TestCreateSessionRejectsInvalidSessionsBaseDomain(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	_, err := repo.CreateSession(ctx, CreateInput{Title: "Bad Base Domain", Slug: "bad-base-domain", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: strings.Repeat("a", 64) + ".example.com"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateSession() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateSessionRejectsDuplicateSlug(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	input := CreateInput{Title: "One", Slug: "same-slug", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"}
	if _, err := repo.CreateSession(ctx, input); err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}
	input.Title = "Two"
	_, err := repo.CreateSession(ctx, input)
	if !errors.Is(err, ErrSlugConflicts) {
		t.Fatalf("second CreateSession() error = %v, want ErrSlugConflicts", err)
	}
}

func TestJoinSessionValidatesHashedTokenAndRecordsUse(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Join Me", Slug: "join-me", DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 13, 0, 0, 0, time.UTC) }

	joined, err := repo.JoinSession(ctx, "join-me", created.GuestToken)
	if err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}
	if joined.Session.ID != created.Session.ID || joined.Token.Role != "guest" {
		t.Fatalf("JoinSession() = %+v", joined)
	}
	if joined.Token.LastUsedAt == nil || *joined.Token.LastUsedAt != "2026-04-24T13:00:00.000000000Z" {
		t.Fatalf("LastUsedAt = %v", joined.Token.LastUsedAt)
	}
	_, err = repo.JoinSession(ctx, "join-me", "wrong-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("JoinSession() invalid token error = %v", err)
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("The Infra Podcast #313"); got != "the-infra-podcast-313" {
		t.Fatalf("Slugify() = %q", got)
	}
	if got := Slugify("---"); got != "session" {
		t.Fatalf("Slugify(empty) = %q", got)
	}
}

func assertRoomDomain(t *testing.T, roomDomain *string, baseDomain string) {
	t.Helper()
	if roomDomain == nil {
		t.Fatal("RoomDomain = nil")
	}
	suffix := "." + baseDomain
	if !strings.HasSuffix(*roomDomain, suffix) {
		t.Fatalf("RoomDomain = %q, want suffix %q", *roomDomain, suffix)
	}
	label := strings.TrimSuffix(*roomDomain, suffix)
	if !strings.HasPrefix(label, roomLabelPrefix) {
		t.Fatalf("RoomDomain label = %q, want prefix %q", label, roomLabelPrefix)
	}
	if len(label) > maxDNSLabelLength {
		t.Fatalf("RoomDomain label length = %d, want <= %d", len(label), maxDNSLabelLength)
	}
	if strings.Contains(label, "_") {
		t.Fatalf("RoomDomain label = %q contains underscore", label)
	}
}

func openSessionTestDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "control-plane.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if _, err := database.Migrate(ctx, db, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return db
}
