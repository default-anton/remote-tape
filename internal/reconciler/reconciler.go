package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/default-anton/remote-tape/internal/provisioning"
	"github.com/default-anton/remote-tape/internal/session"
)

const (
	defaultInterval  = 5 * time.Second
	defaultBatchSize = 25
)

type Reconciler struct {
	repo        provisioningStore
	provisioner sessionServerManager
	logger      *slog.Logger
	interval    time.Duration
	batchSize   int
}

type Options struct {
	Interval  time.Duration
	BatchSize int
}

type provisioningStore interface {
	ListProvisioningCandidates(ctx context.Context, limit int) ([]session.Session, error)
	ListProvisioningSessions(ctx context.Context, limit int) ([]session.Session, error)
	ListTearingDownSessions(ctx context.Context, limit int) ([]session.Session, error)
	MarkProvisioningStarted(ctx context.Context, sessionID string) (bool, error)
	AssignDroplet(ctx context.Context, sessionID string, dropletID string, dropletIP string, adopted bool) (session.DropletAssignmentResult, error)
	MarkProvisioningFailed(ctx context.Context, sessionID string, cause error) (bool, error)
	MarkForceDestroyed(ctx context.Context, sessionID string, dropletID string) (bool, error)
	MarkForceDestroyFailed(ctx context.Context, sessionID string, cause error) (bool, error)
}

type sessionServerManager interface {
	provisioning.Provisioner
	provisioning.Destroyer
}

func New(repo *session.Repository, provisioner sessionServerManager, logger *slog.Logger, opts Options) *Reconciler {
	return newReconciler(repo, provisioner, logger, opts)
}

func newReconciler(repo provisioningStore, provisioner sessionServerManager, logger *slog.Logger, opts Options) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	return &Reconciler{repo: repo, provisioner: provisioner, logger: logger, interval: opts.Interval, batchSize: opts.BatchSize}
}

func (r *Reconciler) Run(ctx context.Context) {
	r.runStep(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runStep(ctx)
		}
	}
}

func (r *Reconciler) Step(ctx context.Context) error {
	candidates, err := r.repo.ListProvisioningCandidates(ctx, r.batchSize)
	if err != nil {
		return err
	}
	var errs []error
	for _, candidate := range candidates {
		changed, err := r.repo.MarkProvisioningStarted(ctx, candidate.ID)
		if err != nil {
			r.logger.ErrorContext(ctx, "provisioning claim failed", "session_id", candidate.ID, "error", err)
			errs = append(errs, fmt.Errorf("claim session %s: %w", candidate.ID, err))
			continue
		}
		if changed {
			r.logger.InfoContext(ctx, "provisioning claimed", "session_id", candidate.ID)
		} else {
			r.logger.DebugContext(ctx, "provisioning candidate already claimed", "session_id", candidate.ID)
		}
	}

	provisioningSessions, err := r.repo.ListProvisioningSessions(ctx, r.batchSize)
	if err != nil {
		return errors.Join(errors.Join(errs...), err)
	}
	for _, s := range provisioningSessions {
		result, err := r.provisioner.EnsureDroplet(ctx, s)
		if err != nil {
			r.logger.ErrorContext(ctx, "session server provisioning failed", "session_id", s.ID, "error", err)
			if _, markErr := r.repo.MarkProvisioningFailed(ctx, s.ID, err); markErr != nil {
				errs = append(errs, fmt.Errorf("mark provisioning failed for session %s: %w", s.ID, markErr))
			}
			continue
		}
		assignment, err := r.repo.AssignDroplet(ctx, s.ID, result.ID, result.IP, result.Adopted)
		if err != nil {
			errs = append(errs, fmt.Errorf("assign droplet for session %s: %w", s.ID, err))
			continue
		}
		if !assignment.Accepted {
			r.logger.WarnContext(ctx, "session server assignment rejected; destroying untracked droplet", "session_id", s.ID, "droplet_id", result.ID, "status", assignment.Status)
			if _, destroyErr := r.provisioner.ForceDestroySessionServer(ctx, session.Session{ID: s.ID, DropletID: &result.ID}); destroyErr != nil {
				errs = append(errs, fmt.Errorf("destroy rejected droplet for session %s: %w", s.ID, destroyErr))
			}
			continue
		}
		if assignment.Changed {
			r.logger.InfoContext(ctx, "session server assigned", "session_id", s.ID, "droplet_id", result.ID, "droplet_ip", result.IP, "adopted", result.Adopted, "status", assignment.Status)
		}
	}

	tearingDownSessions, err := r.repo.ListTearingDownSessions(ctx, r.batchSize)
	if err != nil {
		return errors.Join(errors.Join(errs...), err)
	}
	for _, s := range tearingDownSessions {
		result, err := r.provisioner.ForceDestroySessionServer(ctx, s)
		if err != nil {
			r.logger.ErrorContext(ctx, "session server force destroy failed", "session_id", s.ID, "error", err)
			if _, markErr := r.repo.MarkForceDestroyFailed(ctx, s.ID, err); markErr != nil {
				errs = append(errs, fmt.Errorf("mark force destroy failed for session %s: %w", s.ID, markErr))
			}
			continue
		}
		changed, err := r.repo.MarkForceDestroyed(ctx, s.ID, result.DropletID)
		if err != nil {
			errs = append(errs, fmt.Errorf("mark force destroyed for session %s: %w", s.ID, err))
			continue
		}
		if changed {
			r.logger.InfoContext(ctx, "session server force destroyed", "session_id", s.ID, "droplet_id", result.DropletID)
		}
	}
	return errors.Join(errs...)
}

func (r *Reconciler) runStep(ctx context.Context) {
	if err := r.Step(ctx); err != nil {
		r.logger.ErrorContext(ctx, "reconciler step failed", "error", err)
	}
}
