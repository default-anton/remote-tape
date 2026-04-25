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
