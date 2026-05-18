package dns

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const RequiredTTL = 60

var (
	ErrConfiguration = errors.New("dns configuration error")
	ErrConflict      = errors.New("dns record conflict")
)

type Manager interface {
	EnsureARecord(ctx context.Context, input EnsureARecordInput) (RecordResult, error)
	DeleteRecord(ctx context.Context, input DeleteRecordInput) error
}

type EnsureARecordInput struct {
	SessionID   string
	RoomDomain  string
	PublicIP    string
	DNSRecordID string
	BaseDomain  string
}

type DeleteRecordInput struct {
	SessionID   string
	RoomDomain  string
	DNSRecordID string
	BaseDomain  string
}

type RecordResult struct {
	ID        string
	ZoneID    string
	Name      string
	Content   string
	Operation string
}

type OperationError struct {
	ZoneID string
	Err    error
}

func (e OperationError) Error() string {
	if e.Err == nil {
		return "dns operation failed"
	}
	return e.Err.Error()
}

func (e OperationError) Unwrap() error {
	return e.Err
}

func ZoneIDFromError(err error) string {
	var opErr OperationError
	if errors.As(err, &opErr) {
		return opErr.ZoneID
	}
	return ""
}

type DisabledManager struct {
	Message string
}

func (m DisabledManager) EnsureARecord(context.Context, EnsureARecordInput) (RecordResult, error) {
	return RecordResult{}, m.err()
}

func (m DisabledManager) DeleteRecord(context.Context, DeleteRecordInput) error {
	return m.err()
}

func (m DisabledManager) err() error {
	message := strings.TrimSpace(m.Message)
	if message == "" {
		message = "cloudflare token missing; set REMOTE_TAPE_CLOUDFLARE_API_TOKEN for live DNS operations"
	}
	return fmt.Errorf("%w: %s", ErrConfiguration, message)
}
