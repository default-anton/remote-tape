package session

import (
	"context"
	"database/sql"
	"encoding/json"
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
		InstanceRegion:     "nyc3",
		InstanceSize:       "s-2vcpu-2gb",
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

func TestListSessionsReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	result, err := repo.ListSessions(ctx, ListSessionsInput{})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if result.Sessions == nil {
		t.Fatal("ListSessions() returned nil slice")
	}
	if len(result.Sessions) != 0 || result.Total != 0 {
		t.Fatalf("ListSessions() result = %+v", result)
	}
}

func TestListSessionsFiltersSortsAndPaginates(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	for _, item := range []struct {
		id        string
		title     string
		slug      string
		status    string
		region    string
		createdAt string
		updatedAt string
	}{
		{"sess_alpha", "Alpha", "alpha", "ready", "nyc3", "2026-04-24T12:00:00.000000000Z", "2026-04-24T12:01:00.000000000Z"},
		{"sess_beta", "Beta", "beta", "active", "sfo2", "2026-04-24T12:02:00.000000000Z", "2026-04-24T12:03:00.000000000Z"},
		{"sess_gamma", "Gamma", "gamma", "ready", "sfo2", "2026-04-24T12:04:00.000000000Z", "2026-04-24T12:05:00.000000000Z"},
	} {
		if _, err := db.ExecContext(ctx, `
insert into sessions(id, slug, title, status, instance_region, instance_size, image_id, created_at, updated_at)
values (?, ?, ?, ?, ?, 's-2vcpu-4gb', 'image', ?, ?);
`, item.id, item.slug, item.title, item.status, item.region, item.createdAt, item.updatedAt); err != nil {
			t.Fatalf("insert session %q: %v", item.id, err)
		}
	}

	result, err := repo.ListSessions(ctx, ListSessionsInput{Page: 1, PageSize: 1, Sort: "title", Direction: "desc", Statuses: []string{"ready"}})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if result.Total != 2 || result.Page != 1 || result.PageSize != 1 || result.PollableCount != 2 {
		t.Fatalf("pagination = %+v", result)
	}
	if len(result.Sessions) != 1 || result.Sessions[0].Slug != "gamma" {
		t.Fatalf("sessions = %+v", result.Sessions)
	}

	result, err = repo.ListSessions(ctx, ListSessionsInput{Page: 99, PageSize: 1, Sort: "title", Direction: "desc", Statuses: []string{"ready"}})
	if err != nil {
		t.Fatalf("ListSessions() clamped page error = %v", err)
	}
	if result.Page != 2 || len(result.Sessions) != 1 || result.Sessions[0].Slug != "alpha" {
		t.Fatalf("clamped result = %+v", result)
	}

	result, err = repo.ListSessions(ctx, ListSessionsInput{Page: 1, PageSize: 10, Regions: []string{"sfo2"}, Query: "bet"})
	if err != nil {
		t.Fatalf("ListSessions() region query error = %v", err)
	}
	if result.Total != 1 || len(result.Sessions) != 1 || result.Sessions[0].Slug != "beta" {
		t.Fatalf("filtered result = %+v", result)
	}

	result, err = repo.ListSessions(ctx, ListSessionsInput{Page: 1, PageSize: 10, Statuses: []string{"ready", "active"}, Regions: []string{"nyc3", "sfo2"}})
	if err != nil {
		t.Fatalf("ListSessions() multi-filter error = %v", err)
	}
	if result.Total != 3 || len(result.Sessions) != 3 {
		t.Fatalf("multi-filter result = %+v", result)
	}

	result, err = repo.ListSessions(ctx, ListSessionsInput{Page: 1, PageSize: 10, Query: "%"})
	if err != nil {
		t.Fatalf("ListSessions() literal wildcard query error = %v", err)
	}
	if result.Total != 0 || len(result.Sessions) != 0 {
		t.Fatalf("literal wildcard result = %+v", result)
	}
}

func TestGetSessionReturnsEmptySlices(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	if _, err := db.ExecContext(ctx, `
insert into sessions(id, slug, title, status, instance_region, instance_size, image_id, created_at, updated_at)
values ('sess_empty', 'empty-detail', 'Empty Detail', 'created', 'nyc3', 's-1vcpu-1gb', 'image', '2026-04-24T12:00:00.000000000Z', '2026-04-24T12:00:00.000000000Z');
`); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	detail, err := repo.GetSession(ctx, "sess_empty")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.AccessTokens == nil {
		t.Fatal("AccessTokens is nil")
	}
	if detail.Events == nil {
		t.Fatal("Events is nil")
	}
}

func TestListAndStreamSessionEventsPreserveOrderAndMetadata(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	if _, err := db.ExecContext(ctx, `
insert into sessions(id, slug, title, status, instance_region, instance_size, image_id, created_at, updated_at)
values ('sess_events', 'events', 'Events', 'created', 'nyc3', 's-1vcpu-1gb', 'image', '2026-04-24T12:00:00.000000000Z', '2026-04-24T12:00:00.000000000Z');
`); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	metadataA := `{"step":1}`
	metadataB := `{not-json}`
	if _, err := AppendEvent(ctx, db, "sess_events", "second", stringPtr("Second"), &metadataB); err != nil {
		t.Fatalf("append second event: %v", err)
	}
	if _, err := AppendEvent(ctx, db, "sess_events", "third", stringPtr("Third"), nil); err != nil {
		t.Fatalf("append third event: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
insert into session_events(session_id, type, message, metadata_json, created_at)
values ('sess_events', 'first', 'First', ?, '2026-04-24T11:59:00.000000000Z');
`, metadataA); err != nil {
		t.Fatalf("insert first event: %v", err)
	}

	listed, err := repo.ListSessionEvents(ctx, "sess_events", ListSessionEventsInput{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if listed.Total != 3 || listed.Page != 1 || listed.PageSize != 2 || len(listed.Events) != 2 || listed.Events[0].Type != "first" || listed.Events[1].Type != "third" {
		t.Fatalf("listed events = %+v", listed)
	}

	pageTwo, err := repo.ListSessionEvents(ctx, "sess_events", ListSessionEventsInput{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("ListSessionEvents(page 2) error = %v", err)
	}
	if pageTwo.Total != 3 || pageTwo.Page != 2 || len(pageTwo.Events) != 1 || pageTwo.Events[0].Type != "second" {
		t.Fatalf("page two events = %+v", pageTwo)
	}
	if pageTwo.Events[0].MetadataJSON == nil || *pageTwo.Events[0].MetadataJSON != metadataB {
		t.Fatalf("listed metadata = %+v", pageTwo.Events[0].MetadataJSON)
	}

	clamped, err := repo.ListSessionEvents(ctx, "sess_events", ListSessionEventsInput{Page: 99, PageSize: 999})
	if err != nil {
		t.Fatalf("ListSessionEvents(clamped) error = %v", err)
	}
	if clamped.Page != 1 || clamped.PageSize != 200 || len(clamped.Events) != 3 {
		t.Fatalf("clamped events = %+v", clamped)
	}

	var exported []Event
	if err := repo.StreamSessionEvents(ctx, "sess_events", func(event Event) error {
		exported = append(exported, event)
		return nil
	}); err != nil {
		t.Fatalf("StreamSessionEvents() error = %v", err)
	}
	if len(exported) != 3 || exported[0].Type != "second" || exported[1].Type != "third" || exported[2].Type != "first" {
		t.Fatalf("exported events = %+v", exported)
	}
	if exported[2].MetadataJSON == nil || *exported[2].MetadataJSON != metadataA {
		t.Fatalf("exported metadata = %+v", exported[2].MetadataJSON)
	}

	if _, err := repo.ListSessionEvents(ctx, "sess_missing", ListSessionEventsInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListSessionEvents() missing error = %v", err)
	}

	called := false
	if err := repo.StreamSessionEvents(ctx, "sess_missing", func(Event) error {
		called = true
		return nil
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("StreamSessionEvents() missing error = %v", err)
	}
	if called {
		t.Fatal("StreamSessionEvents() called callback for missing session")
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
insert into sessions(id, slug, title, status, instance_region, instance_size, image_id, created_at, updated_at)
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Start Provisioning", Slug: "start-provisioning", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Noop Started", Slug: "noop-started", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Fail Provisioning", Slug: "fail-provisioning", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Noop Failed", Slug: "noop-failed", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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

func TestAssignInstanceTransitionsAndWaitingForIP(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Assign Instance", Slug: "assign-instance", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}

	assignment, err := repo.AssignInstance(ctx, created.Session.ID, "123", "", false)
	if err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignInstance(no ip) assignment=%+v error=%v", assignment, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "provisioning" || detail.Session.InstanceID == nil || *detail.Session.InstanceID != "123" || detail.Session.PublicIP != nil {
		t.Fatalf("session after no-ip assign = %+v", detail.Session)
	}
	if detail.Events[len(detail.Events)-1].Type != "provisioning.waiting_for_ip" {
		t.Fatalf("last event = %+v", detail.Events[len(detail.Events)-1])
	}

	assignment, err = repo.AssignInstance(ctx, created.Session.ID, "123", "", false)
	if err != nil {
		t.Fatalf("AssignInstance(repeated no ip) error = %v", err)
	}
	if assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignInstance repeated no-ip assignment = %+v", assignment)
	}
	detail, err = repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.Events) != 3 {
		t.Fatalf("repeated no-ip assignment appended events = %+v", detail.Events)
	}

	assignment, err = repo.AssignInstance(ctx, created.Session.ID, "123", "203.0.113.10", true)
	if err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignInstance(with ip) assignment=%+v error=%v", assignment, err)
	}
	detail, err = repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "waiting_for_dns" || detail.Session.PublicIP == nil || *detail.Session.PublicIP != "203.0.113.10" {
		t.Fatalf("session after ip assign = %+v", detail.Session)
	}
	if detail.Events[len(detail.Events)-1].Type != "provisioning.instance_adopted" {
		t.Fatalf("last event = %+v", detail.Events[len(detail.Events)-1])
	}

	assignment, err = repo.AssignInstance(ctx, created.Session.ID, "123", "203.0.113.10", true)
	if err != nil {
		t.Fatalf("AssignInstance(noop) error = %v", err)
	}
	if assignment.Changed || assignment.Accepted {
		t.Fatalf("AssignInstance outside provisioning assignment = %+v", assignment)
	}
}

func TestListDNSCandidatesReturnsWaitingSessionsWithDomainAndIP(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)

	ready := createWaitingForDNS(t, ctx, repo, "Ready DNS", "ready-dns", "2026-04-24T12:00:00.000000000Z")
	missingIP, err := repo.CreateSession(ctx, CreateInput{Title: "Missing IP", Slug: "missing-ip", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
	if err != nil {
		t.Fatalf("CreateSession(missing ip) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set status = 'waiting_for_dns', public_ip = null, updated_at = '2026-04-24T11:00:00.000000000Z' where id = ?`, missingIP.Session.ID); err != nil {
		t.Fatalf("seed missing ip: %v", err)
	}
	missingDomain, err := repo.CreateSession(ctx, CreateInput{Title: "Missing Domain", Slug: "missing-domain", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession(missing domain) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set status = 'waiting_for_dns', public_ip = '203.0.113.20', updated_at = '2026-04-24T10:00:00.000000000Z' where id = ?`, missingDomain.Session.ID); err != nil {
		t.Fatalf("seed missing domain: %v", err)
	}

	candidates, err := repo.ListDNSCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("ListDNSCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != ready.Session.ID {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestMarkDNSConfiguredClearsErrorAndAppendsMetadata(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created := createWaitingForDNS(t, ctx, repo, "DNS Configured", "dns-configured", "2026-04-24T12:00:00.000000000Z")
	if _, err := db.ExecContext(ctx, `update sessions set last_error = 'old dns error', last_error_at = updated_at, last_error_phase = 'dns', dns_attempts = 2 where id = ?`, created.Session.ID); err != nil {
		t.Fatalf("seed dns error: %v", err)
	}
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 17, 0, 0, 0, time.UTC) }

	changed, err := repo.MarkDNSConfigured(ctx, created.Session.ID, "dns_123", DNSConfiguredMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10", Operation: "created", ZoneID: "zone_123"})
	if err != nil || !changed {
		t.Fatalf("MarkDNSConfigured() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "waiting_for_dns" || detail.Session.DNSRecordID == nil || *detail.Session.DNSRecordID != "dns_123" || detail.Session.LastError != nil || detail.Session.LastErrorAt != nil || detail.Session.LastErrorPhase != nil {
		t.Fatalf("session after dns configured = %+v", detail.Session)
	}
	last := detail.Events[len(detail.Events)-1]
	if last.Type != "dns.configured" || last.MetadataJSON == nil {
		t.Fatalf("last event = %+v", last)
	}
	metadata := map[string]string{}
	if err := json.Unmarshal([]byte(*last.MetadataJSON), &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if metadata["dns_record_id"] != "dns_123" || metadata["operation"] != "created" || metadata["zone_id"] != "zone_123" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestMarkDNSFailedIncrementsAttemptsAndKeepsWaiting(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created := createWaitingForDNS(t, ctx, repo, "DNS Failed", "dns-failed", "2026-04-24T12:00:00.000000000Z")
	repo.now = func() time.Time { return time.Date(2026, 4, 24, 17, 30, 0, 0, time.UTC) }

	changed, err := repo.MarkDNSFailed(ctx, created.Session.ID, errors.New("cloudflare unavailable"), DNSFailureMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10", Operation: "ensure"})
	if err != nil || !changed {
		t.Fatalf("MarkDNSFailed() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "waiting_for_dns" || detail.Session.DNSAttempts != 1 || detail.Session.LastError == nil || *detail.Session.LastError != "cloudflare unavailable" || detail.Session.LastErrorPhase == nil || *detail.Session.LastErrorPhase != "dns" {
		t.Fatalf("session after dns failed = %+v", detail.Session)
	}
	last := detail.Events[len(detail.Events)-1]
	if last.Type != "dns.failed" || last.MetadataJSON == nil || !strings.Contains(*last.MetadataJSON, "cloudflare unavailable") {
		t.Fatalf("last event = %+v", last)
	}
}

func TestMarkDNSDeletedClearsRecordIDAndAppendsEvent(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created := createWaitingForDNS(t, ctx, repo, "DNS Deleted", "dns-deleted", "2026-04-24T12:00:00.000000000Z")
	if changed, err := repo.MarkDNSConfigured(ctx, created.Session.ID, "dns_delete", DNSConfiguredMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10", Operation: "created"}); err != nil || !changed {
		t.Fatalf("MarkDNSConfigured() changed=%v error=%v", changed, err)
	}

	changed, err := repo.MarkDNSDeleted(ctx, created.Session.ID, DNSDeletedMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10", DNSRecordID: "dns_delete", Operation: "deleted"})
	if err != nil || !changed {
		t.Fatalf("MarkDNSDeleted() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.DNSRecordID != nil || detail.Events[len(detail.Events)-1].Type != "dns.deleted" {
		t.Fatalf("session after dns deleted = %+v events=%+v", detail.Session, detail.Events)
	}
}

func TestDNSTransitionsNoOpOutsideAllowedStatuses(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "DNS Noop", Slug: "dns-noop", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkDNSConfigured(ctx, created.Session.ID, "dns_noop", DNSConfiguredMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10"}); err != nil || changed {
		t.Fatalf("MarkDNSConfigured() changed=%v error=%v", changed, err)
	}
	if changed, err := repo.MarkDNSFailed(ctx, created.Session.ID, errors.New("boom"), DNSFailureMetadata{RoomDomain: *created.Session.RoomDomain, PublicIP: "203.0.113.10"}); err != nil || changed {
		t.Fatalf("MarkDNSFailed() changed=%v error=%v", changed, err)
	}
	if changed, err := repo.MarkDNSDeleted(ctx, created.Session.ID, DNSDeletedMetadata{RoomDomain: *created.Session.RoomDomain}); err != nil || changed {
		t.Fatalf("MarkDNSDeleted() changed=%v error=%v", changed, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "created" || detail.Session.DNSRecordID != nil || detail.Session.DNSAttempts != 0 || len(detail.Events) != 1 {
		t.Fatalf("unexpected noop state = %+v events=%+v", detail.Session, detail.Events)
	}
}

func TestForceDestroyLifecycle(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Force Destroy", Slug: "force-destroy", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	if assignment, err := repo.AssignInstance(ctx, created.Session.ID, "123", "203.0.113.10", false); err != nil || !assignment.Changed {
		t.Fatalf("AssignInstance() assignment=%+v error=%v", assignment, err)
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
	if assignment, err := repo.AssignInstance(ctx, created.Session.ID, "456", "203.0.113.11", false); err != nil || !assignment.Changed || !assignment.Accepted {
		t.Fatalf("AssignInstance while tearing_down assignment=%+v error=%v", assignment, err)
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Force Destroy Failure", Slug: "force-destroy-failure", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Needs Machine Token", Slug: "needs-machine-token", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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

func TestIssueMachineTokenRotatesBeforeInstanceAssignment(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Rotate Machine Token", Slug: "rotate-machine-token", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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

func TestIssueMachineTokenRejectsAfterInstanceAssignment(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Locked Machine Token", Slug: "locked-machine-token", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `update sessions set instance_id = 'do-123' where id = ?`, created.Session.ID); err != nil {
		t.Fatalf("set instance id: %v", err)
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

	created, err := repo.CreateSession(ctx, CreateInput{Title: "Max Slug", Slug: slug, InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
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

	_, err := repo.CreateSession(ctx, CreateInput{Title: "Bad Base Domain", Slug: "bad-base-domain", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: strings.Repeat("a", 64) + ".example.com"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateSession() error = %v, want ErrInvalidInput", err)
	}
}

func TestCreateSessionRejectsDuplicateSlug(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	input := CreateInput{Title: "One", Slug: "same-slug", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"}
	if _, err := repo.CreateSession(ctx, input); err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}
	input.Title = "Two"
	_, err := repo.CreateSession(ctx, input)
	if !errors.Is(err, ErrSlugConflicts) {
		t.Fatalf("second CreateSession() error = %v, want ErrSlugConflicts", err)
	}
}

func TestCheckSlugAvailability(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	if _, err := repo.CreateSession(ctx, CreateInput{Title: "Taken", Slug: "taken-slug", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	tests := []struct {
		name       string
		slug       string
		wantSlug   string
		available  bool
		valid      bool
		wantReason *string
	}{
		{name: "available", slug: " New-Slug ", wantSlug: "new-slug", available: true, valid: true},
		{name: "taken", slug: "taken-slug", wantSlug: "taken-slug", available: false, valid: true, wantReason: stringPtr("taken")},
		{name: "invalid", slug: "-bad-", wantSlug: "-bad-", available: false, valid: false, wantReason: stringPtr("invalid_format")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.CheckSlugAvailability(ctx, tt.slug)
			if err != nil {
				t.Fatalf("CheckSlugAvailability() error = %v", err)
			}
			if got.Slug != tt.slug || got.NormalizedSlug != tt.wantSlug || got.Available != tt.available || got.Valid != tt.valid {
				t.Fatalf("CheckSlugAvailability() = %+v", got)
			}
			if !equalStringPtr(got.Reason, tt.wantReason) {
				t.Fatalf("Reason = %v, want %v", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestJoinSessionValidatesHashedTokenAndRecordsUse(t *testing.T) {
	ctx := context.Background()
	db := openSessionTestDB(t, ctx)
	repo := NewRepository(db)
	created, err := repo.CreateSession(ctx, CreateInput{Title: "Join Me", Slug: "join-me", InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
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

func equalStringPtr(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func createWaitingForDNS(t *testing.T, ctx context.Context, repo *Repository, title string, slug string, updatedAt string) CreateResult {
	t.Helper()
	created, err := repo.CreateSession(ctx, CreateInput{Title: title, Slug: slug, InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	if assignment, err := repo.AssignInstance(ctx, created.Session.ID, "123", "203.0.113.10", false); err != nil || !assignment.Changed {
		t.Fatalf("AssignInstance() assignment=%+v error=%v", assignment, err)
	}
	if updatedAt != "" {
		if _, err := repo.db.ExecContext(ctx, `update sessions set updated_at = ? where id = ?`, updatedAt, created.Session.ID); err != nil {
			t.Fatalf("set updated_at: %v", err)
		}
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	created.Session = detail.Session
	return created
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
