package reconciler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/default-anton/remote-tape/internal/database"
	"github.com/default-anton/remote-tape/internal/dns"
	"github.com/default-anton/remote-tape/internal/provisioning"
	"github.com/default-anton/remote-tape/internal/session"
)

func TestStepMovesCreatedSessionsToProvisioning(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	first := createTestSession(t, ctx, repo, "First", "first")
	second := createTestSession(t, ctx, repo, "Second", "second")

	r := New(repo, fakeProvisioner{}, nil, discardLogger(), Options{})
	if err := r.Step(ctx); err != nil {
		t.Fatalf("Step() error = %v", err)
	}

	assertStatus(t, ctx, repo, first.Session.ID, "waiting_for_dns", 1, 3)
	assertStatus(t, ctx, repo, second.Session.ID, "waiting_for_dns", 1, 3)
}

func TestStepUsesConfiguredBatchSize(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{candidates: []session.Session{{ID: "sess_one"}, {ID: "sess_two"}, {ID: "sess_three"}}}
	r := newReconciler(store, fakeProvisioner{}, nil, discardLogger(), Options{BatchSize: 2})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("Step() error = %v", err)
	}
	if got := strings.Join(store.started, ","); got != "sess_one,sess_two" {
		t.Fatalf("started = %q", got)
	}
}

func TestStepIsIdempotentForAlreadyClaimedSessions(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	created := createTestSession(t, ctx, repo, "Once", "once")
	r := New(repo, fakeProvisioner{}, nil, discardLogger(), Options{})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("first Step() error = %v", err)
	}
	if err := r.Step(ctx); err != nil {
		t.Fatalf("second Step() error = %v", err)
	}
	assertStatus(t, ctx, repo, created.Session.ID, "waiting_for_dns", 1, 3)
}

func TestStepEnsuresDNSForWaitingSession(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	created := createDNSCandidate(t, ctx, repo, "DNS", "dns")
	dnsManager := &fakeDNSManager{result: dns.RecordResult{ID: "dns_123", ZoneID: "zone_123", Name: *created.Session.RoomDomain, Content: "203.0.113.10", Operation: "created"}}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("Step() error = %v", err)
	}
	if len(dnsManager.ensureInputs) != 1 || dnsManager.ensureInputs[0].RoomDomain != *created.Session.RoomDomain || dnsManager.ensureInputs[0].PublicIP != "203.0.113.10" {
		t.Fatalf("ensure inputs = %+v", dnsManager.ensureInputs)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "waiting_for_dns" || detail.Session.DNSRecordID == nil || *detail.Session.DNSRecordID != "dns_123" || detail.Events[len(detail.Events)-1].Type != "dns.configured" {
		t.Fatalf("session after dns = %+v events=%+v", detail.Session, detail.Events)
	}
}

func TestStepPersistsDNSFailureAndContinuesCandidates(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	bad := createDNSCandidate(t, ctx, repo, "Bad DNS", "bad-dns")
	good := createDNSCandidate(t, ctx, repo, "Good DNS", "good-dns")
	dnsManager := &fakeDNSManager{
		result: dns.RecordResult{ID: "dns_good", ZoneID: "zone_123", Name: *good.Session.RoomDomain, Content: "203.0.113.10", Operation: "created"},
		failForRoom: map[string]error{
			*bad.Session.RoomDomain: errors.New("cloudflare timeout"),
		},
	}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	err := r.Step(ctx)
	if err == nil || !strings.Contains(err.Error(), "cloudflare timeout") {
		t.Fatalf("Step() error = %v", err)
	}
	badDetail, err := repo.GetSession(ctx, bad.Session.ID)
	if err != nil {
		t.Fatalf("GetSession(bad) error = %v", err)
	}
	if badDetail.Session.Status != "waiting_for_dns" || badDetail.Session.DNSAttempts != 1 || badDetail.Session.LastErrorPhase == nil || *badDetail.Session.LastErrorPhase != "dns" {
		t.Fatalf("bad session = %+v", badDetail.Session)
	}
	goodDetail, err := repo.GetSession(ctx, good.Session.ID)
	if err != nil {
		t.Fatalf("GetSession(good) error = %v", err)
	}
	if goodDetail.Session.DNSRecordID == nil || *goodDetail.Session.DNSRecordID != "dns_good" {
		t.Fatalf("good session = %+v", goodDetail.Session)
	}
}

func TestStepDNSConfiguredSecondPassIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	created := createDNSCandidate(t, ctx, repo, "DNS Twice", "dns-twice")
	dnsManager := &fakeDNSManager{result: dns.RecordResult{ID: "dns_once", ZoneID: "zone_123", Name: *created.Session.RoomDomain, Content: "203.0.113.10", Operation: "created"}}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("first Step() error = %v", err)
	}
	dnsManager.result.Operation = "adopted"
	if err := r.Step(ctx); err != nil {
		t.Fatalf("second Step() error = %v", err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	dnsEvents := 0
	for _, event := range detail.Events {
		if event.Type == "dns.configured" {
			dnsEvents++
		}
	}
	if dnsEvents != 1 || len(dnsManager.ensureInputs) != 2 || dnsManager.ensureInputs[1].DNSRecordID != "dns_once" {
		t.Fatalf("dnsEvents=%d ensureInputs=%+v", dnsEvents, dnsManager.ensureInputs)
	}
}

func TestStepDNSFailureDoesNotStopForceDestroy(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	candidate := createDNSCandidate(t, ctx, repo, "DNS Fails", "dns-fails")
	teardown := createDNSCandidate(t, ctx, repo, "Teardown", "teardown")
	if changed, err := repo.MarkDNSConfigured(ctx, teardown.Session.ID, "dns_teardown", session.DNSConfiguredMetadata{RoomDomain: *teardown.Session.RoomDomain, PublicIP: "203.0.113.10", Operation: "created"}); err != nil || !changed {
		t.Fatalf("MarkDNSConfigured() changed=%v error=%v", changed, err)
	}
	if changed, err := repo.MarkForceDestroyStarted(ctx, teardown.Session.ID); err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	dnsManager := &fakeDNSManager{
		result: dns.RecordResult{ID: "unused", Name: *candidate.Session.RoomDomain, Content: "203.0.113.10", Operation: "created"},
		failForRoom: map[string]error{
			*candidate.Session.RoomDomain: errors.New("cloudflare down"),
		},
	}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	err := r.Step(ctx)
	if err == nil || !strings.Contains(err.Error(), "cloudflare down") {
		t.Fatalf("Step() error = %v", err)
	}
	detail, err := repo.GetSession(ctx, teardown.Session.ID)
	if err != nil {
		t.Fatalf("GetSession(teardown) error = %v", err)
	}
	if detail.Session.Status != "ended" {
		t.Fatalf("teardown session = %+v", detail.Session)
	}
}

func TestStepDeletesDNSByLookupBeforeForceDestroy(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	teardown := createDNSCandidate(t, ctx, repo, "Lookup Delete", "lookup-delete")
	if changed, err := repo.MarkForceDestroyStarted(ctx, teardown.Session.ID); err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	dnsManager := &fakeDNSManager{}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("Step() error = %v", err)
	}
	if len(dnsManager.deleteInputs) != 1 || dnsManager.deleteInputs[0].DNSRecordID != "" || dnsManager.deleteInputs[0].RoomDomain != *teardown.Session.RoomDomain {
		t.Fatalf("delete inputs = %+v", dnsManager.deleteInputs)
	}
	detail, err := repo.GetSession(ctx, teardown.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "ended" {
		t.Fatalf("teardown session = %+v", detail.Session)
	}
}

func TestStepDNSDeleteFailureBlocksForceDestroy(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	teardown := createDNSCandidate(t, ctx, repo, "Delete Fails", "delete-fails")
	if changed, err := repo.MarkDNSConfigured(ctx, teardown.Session.ID, "dns_delete", session.DNSConfiguredMetadata{RoomDomain: *teardown.Session.RoomDomain, PublicIP: "203.0.113.10", Operation: "created"}); err != nil || !changed {
		t.Fatalf("MarkDNSConfigured() changed=%v error=%v", changed, err)
	}
	if changed, err := repo.MarkForceDestroyStarted(ctx, teardown.Session.ID); err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	dnsManager := &fakeDNSManager{deleteErr: errors.New("cloudflare delete failed")}
	r := New(repo, fakeProvisioner{}, dnsManager, discardLogger(), Options{SessionsBaseDomain: "sessions.example.com"})

	err := r.Step(ctx)
	if err == nil || !strings.Contains(err.Error(), "cloudflare delete failed") {
		t.Fatalf("Step() error = %v", err)
	}
	detail, err := repo.GetSession(ctx, teardown.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "tearing_down" || detail.Session.LastErrorPhase == nil || *detail.Session.LastErrorPhase != "dns" || detail.Session.DNSAttempts != 1 {
		t.Fatalf("teardown session = %+v", detail.Session)
	}
}

func TestStepDestroysInstanceCreatedAfterForceDestroyRequest(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	created := createTestSession(t, ctx, repo, "Race", "race")
	provisioner := newBlockingProvisioner(provisioning.InstanceResult{ID: "789", IP: "203.0.113.89"})
	r := New(repo, provisioner, nil, discardLogger(), Options{})
	done := make(chan error, 1)

	go func() {
		done <- r.Step(ctx)
	}()

	select {
	case <-provisioner.ensureStarted:
	case <-time.After(time.Second):
		t.Fatal("EnsureInstance was not called")
	}
	if changed, err := repo.MarkForceDestroyStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkForceDestroyStarted() changed=%v error=%v", changed, err)
	}
	close(provisioner.releaseEnsure)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Step() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Step() did not finish")
	}

	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != "ended" || detail.Session.InstanceID == nil || *detail.Session.InstanceID != "789" {
		t.Fatalf("session after race = %+v", detail.Session)
	}
	if got := provisioner.destroyedID(); got != "789" {
		t.Fatalf("destroyed instance id = %q", got)
	}
}

func TestStepKeepsProcessingAfterCandidateFailure(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		candidates: []session.Session{{ID: "sess_bad"}, {ID: "sess_good"}},
		fail:       map[string]error{"sess_bad": errors.New("database busy")},
	}
	r := newReconciler(store, fakeProvisioner{}, nil, discardLogger(), Options{})

	err := r.Step(ctx)
	if err == nil || !strings.Contains(err.Error(), "sess_bad") {
		t.Fatalf("Step() error = %v", err)
	}
	if got := strings.Join(store.started, ","); got != "sess_bad,sess_good" {
		t.Fatalf("started = %q", got)
	}
}

func TestRunStepsImmediatelyAndThenTicksUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &fakeStore{candidates: []session.Session{{ID: "sess_one"}}}
	r := newReconciler(store, fakeProvisioner{}, nil, discardLogger(), Options{Interval: time.Hour})
	done := make(chan struct{})

	go func() {
		r.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool { return store.stepCalls() >= 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after context cancellation")
	}
}

func TestRunLogsStepErrorsAndContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &fakeStore{
		candidates: []session.Session{{ID: "sess_flaky"}},
		fail:       map[string]error{"sess_flaky": errors.New("temporary failure")},
	}
	r := newReconciler(store, fakeProvisioner{}, nil, discardLogger(), Options{Interval: time.Millisecond})
	done := make(chan struct{})

	go func() {
		r.Run(ctx)
		close(done)
	}()

	waitFor(t, func() bool { return store.stepCalls() >= 2 })
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop")
	}
}

type fakeStore struct {
	mu         sync.Mutex
	candidates []session.Session
	fail       map[string]error
	started    []string
	lists      int
}

func (s *fakeStore) ListProvisioningCandidates(_ context.Context, limit int) ([]session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists++
	candidates := s.candidates
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return append([]session.Session(nil), candidates...), nil
}

func (s *fakeStore) MarkProvisioningStarted(_ context.Context, sessionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = append(s.started, sessionID)
	if err := s.fail[sessionID]; err != nil {
		return false, err
	}
	return true, nil
}

func (s *fakeStore) ListProvisioningSessions(context.Context, int) ([]session.Session, error) {
	return nil, nil
}

func (s *fakeStore) ListDNSCandidates(context.Context, int) ([]session.Session, error) {
	return nil, nil
}

func (s *fakeStore) ListTearingDownSessions(context.Context, int) ([]session.Session, error) {
	return nil, nil
}

func (s *fakeStore) AssignInstance(context.Context, string, string, string, bool) (session.InstanceAssignmentResult, error) {
	return session.InstanceAssignmentResult{Accepted: true, Changed: true, Status: "waiting_for_dns"}, nil
}

func (s *fakeStore) MarkProvisioningFailed(context.Context, string, error) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkDNSConfigured(context.Context, string, string, session.DNSConfiguredMetadata) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkDNSFailed(context.Context, string, error, session.DNSFailureMetadata) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkDNSDeleted(context.Context, string, session.DNSDeletedMetadata) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkForceDestroyed(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkForceDestroyFailed(context.Context, string, error) (bool, error) {
	return true, nil
}

type fakeDNSManager struct {
	result       dns.RecordResult
	err          error
	deleteErr    error
	failForRoom  map[string]error
	ensureInputs []dns.EnsureARecordInput
	deleteInputs []dns.DeleteRecordInput
}

func (m *fakeDNSManager) EnsureARecord(_ context.Context, input dns.EnsureARecordInput) (dns.RecordResult, error) {
	m.ensureInputs = append(m.ensureInputs, input)
	if err := m.failForRoom[input.RoomDomain]; err != nil {
		return dns.RecordResult{}, err
	}
	if m.err != nil {
		return dns.RecordResult{}, m.err
	}
	result := m.result
	if result.Name == "" {
		result.Name = input.RoomDomain
	}
	if result.Content == "" {
		result.Content = input.PublicIP
	}
	if result.ID == "" {
		result.ID = input.DNSRecordID
	}
	if result.Operation == "" {
		result.Operation = "adopted"
	}
	return result, nil
}

func (m *fakeDNSManager) DeleteRecord(_ context.Context, input dns.DeleteRecordInput) error {
	m.deleteInputs = append(m.deleteInputs, input)
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return m.err
}

type fakeProvisioner struct{}

func (fakeProvisioner) EnsureInstance(context.Context, session.Session) (provisioning.InstanceResult, error) {
	return provisioning.InstanceResult{ID: "123", IP: "203.0.113.10"}, nil
}

func (fakeProvisioner) ForceDestroySessionServer(context.Context, session.Session) (provisioning.DestroyResult, error) {
	return provisioning.DestroyResult{InstanceID: "123"}, nil
}

type blockingProvisioner struct {
	result        provisioning.InstanceResult
	ensureStarted chan struct{}
	releaseEnsure chan struct{}
	mu            sync.Mutex
	destroyed     string
}

func newBlockingProvisioner(result provisioning.InstanceResult) *blockingProvisioner {
	return &blockingProvisioner{result: result, ensureStarted: make(chan struct{}), releaseEnsure: make(chan struct{})}
}

func (p *blockingProvisioner) EnsureInstance(context.Context, session.Session) (provisioning.InstanceResult, error) {
	close(p.ensureStarted)
	<-p.releaseEnsure
	return p.result, nil
}

func (p *blockingProvisioner) ForceDestroySessionServer(_ context.Context, s session.Session) (provisioning.DestroyResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.InstanceID != nil {
		p.destroyed = *s.InstanceID
	}
	return provisioning.DestroyResult{InstanceID: p.destroyed}, nil
}

func (p *blockingProvisioner) destroyedID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.destroyed
}

func (s *fakeStore) stepCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lists
}

func openReconcilerTestRepo(t *testing.T, ctx context.Context) *session.Repository {
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
	if _, err := database.Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return session.NewRepository(db)
}

func createTestSession(t *testing.T, ctx context.Context, repo *session.Repository, title string, slug string) session.CreateResult {
	t.Helper()
	created, err := repo.CreateSession(ctx, session.CreateInput{Title: title, Slug: slug, InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return created
}

func createDNSCandidate(t *testing.T, ctx context.Context, repo *session.Repository, title string, slug string) session.CreateResult {
	t.Helper()
	created, err := repo.CreateSession(ctx, session.CreateInput{Title: title, Slug: slug, InstanceRegion: "nyc3", InstanceSize: "s-1vcpu-1gb", ImageID: "image", SessionsBaseDomain: "sessions.example.com"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if changed, err := repo.MarkProvisioningStarted(ctx, created.Session.ID); err != nil || !changed {
		t.Fatalf("MarkProvisioningStarted() changed=%v error=%v", changed, err)
	}
	if assignment, err := repo.AssignInstance(ctx, created.Session.ID, "123", "203.0.113.10", false); err != nil || !assignment.Changed {
		t.Fatalf("AssignInstance() assignment=%+v error=%v", assignment, err)
	}
	detail, err := repo.GetSession(ctx, created.Session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	created.Session = detail.Session
	return created
}

func assertStatus(t *testing.T, ctx context.Context, repo *session.Repository, id string, status string, attempts int64, eventCount int) {
	t.Helper()
	detail, err := repo.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if detail.Session.Status != status || detail.Session.ProvisionAttempts != attempts || len(detail.Events) != eventCount {
		t.Fatalf("session status=%q attempts=%d events=%d, want status=%q attempts=%d events=%d", detail.Session.Status, detail.Session.ProvisionAttempts, len(detail.Events), status, attempts, eventCount)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
