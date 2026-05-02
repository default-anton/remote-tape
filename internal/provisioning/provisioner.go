package provisioning

import (
	"context"

	"github.com/default-anton/remote-tape/internal/session"
)

type InstanceResult struct {
	ID      string
	IP      string
	Adopted bool
}

type InstanceProvider interface {
	EnsureInstance(ctx context.Context, s session.Session) (InstanceResult, error)
}

type Destroyer interface {
	ForceDestroySessionServer(ctx context.Context, s session.Session) (DestroyResult, error)
}

type DestroyResult struct {
	InstanceID string
}
