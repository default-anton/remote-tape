package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/default-anton/remote-tape/internal/dns"
	"github.com/default-anton/remote-tape/internal/provisioning"
	"github.com/default-anton/remote-tape/internal/session"
)

const (
	defaultInterval  = 5 * time.Second
	defaultBatchSize = 25
)

type Reconciler struct {
	repo               store
	provisioner        sessionServerManager
	dnsManager         dns.Manager
	logger             *slog.Logger
	interval           time.Duration
	batchSize          int
	sessionsBaseDomain string
}

type Options struct {
	Interval           time.Duration
	BatchSize          int
	SessionsBaseDomain string
}

type store interface {
	ListProvisioningCandidates(ctx context.Context, limit int) ([]session.Session, error)
	ListProvisioningSessions(ctx context.Context, limit int) ([]session.Session, error)
	ListDNSCandidates(ctx context.Context, limit int) ([]session.Session, error)
	ListTearingDownSessions(ctx context.Context, limit int) ([]session.Session, error)
	MarkProvisioningStarted(ctx context.Context, sessionID string) (bool, error)
	AssignInstance(ctx context.Context, sessionID string, instanceID string, publicIP string, adopted bool) (session.InstanceAssignmentResult, error)
	MarkProvisioningFailed(ctx context.Context, sessionID string, cause error) (bool, error)
	MarkDNSConfigured(ctx context.Context, sessionID string, dnsRecordID string, metadata session.DNSConfiguredMetadata) (bool, error)
	MarkDNSFailed(ctx context.Context, sessionID string, cause error, metadata session.DNSFailureMetadata) (bool, error)
	MarkDNSDeleted(ctx context.Context, sessionID string, metadata session.DNSDeletedMetadata) (bool, error)
	MarkForceDestroyed(ctx context.Context, sessionID string, instanceID string) (bool, error)
	MarkForceDestroyFailed(ctx context.Context, sessionID string, cause error) (bool, error)
}

type sessionServerManager interface {
	provisioning.InstanceProvider
	provisioning.Destroyer
}

func New(repo *session.Repository, provisioner sessionServerManager, dnsManager dns.Manager, logger *slog.Logger, opts Options) *Reconciler {
	return newReconciler(repo, provisioner, dnsManager, logger, opts)
}

func newReconciler(repo store, provisioner sessionServerManager, dnsManager dns.Manager, logger *slog.Logger, opts Options) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if dnsManager == nil {
		dnsManager = dns.DisabledManager{}
	}
	return &Reconciler{
		repo:               repo,
		provisioner:        provisioner,
		dnsManager:         dnsManager,
		logger:             logger,
		interval:           opts.Interval,
		batchSize:          opts.BatchSize,
		sessionsBaseDomain: opts.SessionsBaseDomain,
	}
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
		result, err := r.provisioner.EnsureInstance(ctx, s)
		if err != nil {
			r.logger.ErrorContext(ctx, "session server provisioning failed", "session_id", s.ID, "error", err)
			if _, markErr := r.repo.MarkProvisioningFailed(ctx, s.ID, err); markErr != nil {
				errs = append(errs, fmt.Errorf("mark provisioning failed for session %s: %w", s.ID, markErr))
			}
			continue
		}
		assignment, err := r.repo.AssignInstance(ctx, s.ID, result.ID, result.IP, result.Adopted)
		if err != nil {
			errs = append(errs, fmt.Errorf("assign instance for session %s: %w", s.ID, err))
			continue
		}
		if !assignment.Accepted {
			r.logger.WarnContext(ctx, "session server assignment rejected; destroying untracked instance", "session_id", s.ID, "instance_id", result.ID, "status", assignment.Status)
			if _, destroyErr := r.provisioner.ForceDestroySessionServer(ctx, session.Session{ID: s.ID, InstanceID: &result.ID}); destroyErr != nil {
				errs = append(errs, fmt.Errorf("destroy rejected instance for session %s: %w", s.ID, destroyErr))
			}
			continue
		}
		if assignment.Changed {
			r.logger.InfoContext(ctx, "session server assigned", "session_id", s.ID, "instance_id", result.ID, "public_ip", result.IP, "adopted", result.Adopted, "status", assignment.Status)
		}
	}

	dnsCandidates, err := r.repo.ListDNSCandidates(ctx, r.batchSize)
	if err != nil {
		return errors.Join(errors.Join(errs...), err)
	}
	for _, s := range dnsCandidates {
		if s.RoomDomain == nil || s.PublicIP == nil {
			continue
		}
		input := dns.EnsureARecordInput{
			SessionID:   s.ID,
			RoomDomain:  *s.RoomDomain,
			PublicIP:    *s.PublicIP,
			DNSRecordID: stringValue(s.DNSRecordID),
			BaseDomain:  r.sessionsBaseDomain,
		}
		result, err := r.dnsManager.EnsureARecord(ctx, input)
		if err != nil {
			zoneID := dns.ZoneIDFromError(err)
			metadata := session.DNSFailureMetadata{
				RoomDomain:  *s.RoomDomain,
				PublicIP:    *s.PublicIP,
				DNSRecordID: stringValue(s.DNSRecordID),
				Operation:   "ensure",
				ZoneID:      zoneID,
			}
			r.logger.ErrorContext(ctx, "dns provisioning failed", "session_id", s.ID, "room_domain", *s.RoomDomain, "public_ip", *s.PublicIP, "zone_id", zoneID, "dns_record_id", stringValue(s.DNSRecordID), "operation", "ensure", "error", err)
			if _, markErr := r.repo.MarkDNSFailed(ctx, s.ID, err, metadata); markErr != nil {
				errs = append(errs, fmt.Errorf("mark dns failed for session %s: %w", s.ID, markErr))
			}
			errs = append(errs, fmt.Errorf("ensure dns for session %s: %w", s.ID, err))
			continue
		}
		metadata := session.DNSConfiguredMetadata{
			RoomDomain:  result.Name,
			PublicIP:    result.Content,
			DNSRecordID: result.ID,
			Operation:   result.Operation,
			ZoneID:      result.ZoneID,
		}
		if _, err := r.repo.MarkDNSConfigured(ctx, s.ID, result.ID, metadata); err != nil {
			errs = append(errs, fmt.Errorf("mark dns configured for session %s: %w", s.ID, err))
			continue
		}
		r.logger.InfoContext(ctx, "dns configured", "session_id", s.ID, "room_domain", result.Name, "public_ip", result.Content, "zone_id", result.ZoneID, "dns_record_id", result.ID, "operation", result.Operation)
	}

	tearingDownSessions, err := r.repo.ListTearingDownSessions(ctx, r.batchSize)
	if err != nil {
		return errors.Join(errors.Join(errs...), err)
	}
	for _, s := range tearingDownSessions {
		roomDomain := stringValue(s.RoomDomain)
		dnsRecordID := stringValue(s.DNSRecordID)
		if strings.TrimSpace(roomDomain) != "" {
			input := dns.DeleteRecordInput{SessionID: s.ID, RoomDomain: roomDomain, DNSRecordID: dnsRecordID, BaseDomain: r.sessionsBaseDomain}
			if err := r.dnsManager.DeleteRecord(ctx, input); err != nil {
				zoneID := dns.ZoneIDFromError(err)
				metadata := session.DNSFailureMetadata{RoomDomain: roomDomain, PublicIP: stringValue(s.PublicIP), DNSRecordID: dnsRecordID, Operation: "delete", ZoneID: zoneID}
				r.logger.ErrorContext(ctx, "dns deletion failed", "session_id", s.ID, "room_domain", roomDomain, "zone_id", zoneID, "dns_record_id", dnsRecordID, "operation", "delete", "error", err)
				if _, markErr := r.repo.MarkDNSFailed(ctx, s.ID, err, metadata); markErr != nil {
					errs = append(errs, fmt.Errorf("mark dns delete failed for session %s: %w", s.ID, markErr))
				}
				errs = append(errs, fmt.Errorf("delete dns for session %s: %w", s.ID, err))
				continue
			}
			metadata := session.DNSDeletedMetadata{RoomDomain: roomDomain, PublicIP: stringValue(s.PublicIP), DNSRecordID: dnsRecordID, Operation: "deleted"}
			if _, err := r.repo.MarkDNSDeleted(ctx, s.ID, metadata); err != nil {
				errs = append(errs, fmt.Errorf("mark dns deleted for session %s: %w", s.ID, err))
				continue
			}
			r.logger.InfoContext(ctx, "dns deleted", "session_id", s.ID, "room_domain", roomDomain, "dns_record_id", dnsRecordID, "operation", "delete")
		}

		result, err := r.provisioner.ForceDestroySessionServer(ctx, s)
		if err != nil {
			r.logger.ErrorContext(ctx, "session server force destroy failed", "session_id", s.ID, "error", err)
			if _, markErr := r.repo.MarkForceDestroyFailed(ctx, s.ID, err); markErr != nil {
				errs = append(errs, fmt.Errorf("mark force destroy failed for session %s: %w", s.ID, markErr))
			}
			continue
		}
		changed, err := r.repo.MarkForceDestroyed(ctx, s.ID, result.InstanceID)
		if err != nil {
			errs = append(errs, fmt.Errorf("mark force destroyed for session %s: %w", s.ID, err))
			continue
		}
		if changed {
			r.logger.InfoContext(ctx, "session server force destroyed", "session_id", s.ID, "instance_id", result.InstanceID)
		}
	}
	return errors.Join(errs...)
}

func (r *Reconciler) runStep(ctx context.Context) {
	if err := r.Step(ctx); err != nil {
		r.logger.ErrorContext(ctx, "reconciler step failed", "error", err)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
