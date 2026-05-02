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
	"github.com/default-anton/remote-tape/internal/provisioning"
	"github.com/default-anton/remote-tape/internal/session"
)

func TestStepMovesCreatedSessionsToProvisioning(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	first := createTestSession(t, ctx, repo, "First", "first")
	second := createTestSession(t, ctx, repo, "Second", "second")

	r := New(repo, fakeProvisioner{}, discardLogger(), Options{})
	if err := r.Step(ctx); err != nil {
		t.Fatalf("Step() error = %v", err)
	}

	assertStatus(t, ctx, repo, first.Session.ID, "waiting_for_dns", 1, 3)
	assertStatus(t, ctx, repo, second.Session.ID, "waiting_for_dns", 1, 3)
}

func TestStepUsesConfiguredBatchSize(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{candidates: []session.Session{{ID: "sess_one"}, {ID: "sess_two"}, {ID: "sess_three"}}}
	r := newReconciler(store, fakeProvisioner{}, discardLogger(), Options{BatchSize: 2})

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
	r := New(repo, fakeProvisioner{}, discardLogger(), Options{})

	if err := r.Step(ctx); err != nil {
		t.Fatalf("first Step() error = %v", err)
	}
	if err := r.Step(ctx); err != nil {
		t.Fatalf("second Step() error = %v", err)
	}
	assertStatus(t, ctx, repo, created.Session.ID, "waiting_for_dns", 1, 3)
}

func TestStepDestroysDropletCreatedAfterForceDestroyRequest(t *testing.T) {
	ctx := context.Background()
	repo := openReconcilerTestRepo(t, ctx)
	created := createTestSession(t, ctx, repo, "Race", "race")
	provisioner := newBlockingProvisioner(provisioning.DropletResult{ID: "789", IP: "203.0.113.89"})
	r := New(repo, provisioner, discardLogger(), Options{})
	done := make(chan error, 1)

	go func() {
		done <- r.Step(ctx)
	}()

	select {
	case <-provisioner.ensureStarted:
	case <-time.After(time.Second):
		t.Fatal("EnsureDroplet was not called")
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
	if detail.Session.Status != "ended" || detail.Session.DropletID == nil || *detail.Session.DropletID != "789" {
		t.Fatalf("session after race = %+v", detail.Session)
	}
	if got := provisioner.destroyedID(); got != "789" {
		t.Fatalf("destroyed droplet id = %q", got)
	}
}

func TestStepKeepsProcessingAfterCandidateFailure(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{
		candidates: []session.Session{{ID: "sess_bad"}, {ID: "sess_good"}},
		fail:       map[string]error{"sess_bad": errors.New("database busy")},
	}
	r := newReconciler(store, fakeProvisioner{}, discardLogger(), Options{})

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
	r := newReconciler(store, fakeProvisioner{}, discardLogger(), Options{Interval: time.Hour})
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
	r := newReconciler(store, fakeProvisioner{}, discardLogger(), Options{Interval: time.Millisecond})
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

func (s *fakeStore) ListTearingDownSessions(context.Context, int) ([]session.Session, error) {
	return nil, nil
}

func (s *fakeStore) AssignDroplet(context.Context, string, string, string, bool) (session.DropletAssignmentResult, error) {
	return session.DropletAssignmentResult{Accepted: true, Changed: true, Status: "waiting_for_dns"}, nil
}

func (s *fakeStore) MarkProvisioningFailed(context.Context, string, error) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkForceDestroyed(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s *fakeStore) MarkForceDestroyFailed(context.Context, string, error) (bool, error) {
	return true, nil
}

type fakeProvisioner struct{}

func (fakeProvisioner) EnsureDroplet(context.Context, session.Session) (provisioning.DropletResult, error) {
	return provisioning.DropletResult{ID: "123", IP: "203.0.113.10"}, nil
}

func (fakeProvisioner) ForceDestroySessionServer(context.Context, session.Session) (provisioning.DestroyResult, error) {
	return provisioning.DestroyResult{DropletID: "123"}, nil
}

type blockingProvisioner struct {
	result        provisioning.DropletResult
	ensureStarted chan struct{}
	releaseEnsure chan struct{}
	mu            sync.Mutex
	destroyed     string
}

func newBlockingProvisioner(result provisioning.DropletResult) *blockingProvisioner {
	return &blockingProvisioner{result: result, ensureStarted: make(chan struct{}), releaseEnsure: make(chan struct{})}
}

func (p *blockingProvisioner) EnsureDroplet(context.Context, session.Session) (provisioning.DropletResult, error) {
	close(p.ensureStarted)
	<-p.releaseEnsure
	return p.result, nil
}

func (p *blockingProvisioner) ForceDestroySessionServer(_ context.Context, s session.Session) (provisioning.DestroyResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.DropletID != nil {
		p.destroyed = *s.DropletID
	}
	return provisioning.DestroyResult{DropletID: p.destroyed}, nil
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
	created, err := repo.CreateSession(ctx, session.CreateInput{Title: title, Slug: slug, DropletRegion: "nyc3", DropletSize: "s-1vcpu-1gb", ImageID: "image"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
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
