package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/default-anton/remote-tape/internal/session"
)

const (
	defaultInterval  = 5 * time.Second
	defaultBatchSize = 25
)

type Reconciler struct {
	repo      provisioningStore
	logger    *slog.Logger
	interval  time.Duration
	batchSize int
}

type Options struct {
	Interval  time.Duration
	BatchSize int
}

type provisioningStore interface {
	ListProvisioningCandidates(ctx context.Context, limit int) ([]session.Session, error)
	MarkProvisioningStarted(ctx context.Context, sessionID string) (bool, error)
}

func New(repo *session.Repository, logger *slog.Logger, opts Options) *Reconciler {
	return newReconciler(repo, logger, opts)
}

func newReconciler(repo provisioningStore, logger *slog.Logger, opts Options) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	return &Reconciler{repo: repo, logger: logger, interval: opts.Interval, batchSize: opts.BatchSize}
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
	return errors.Join(errs...)
}

func (r *Reconciler) runStep(ctx context.Context) {
	if err := r.Step(ctx); err != nil {
		r.logger.ErrorContext(ctx, "reconciler step failed", "error", err)
	}
}
