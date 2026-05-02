package provisioning

import (
	"context"

	"github.com/default-anton/remote-tape/internal/session"
)

type DropletResult struct {
	ID      string
	IP      string
	Adopted bool
}

type Provisioner interface {
	EnsureDroplet(ctx context.Context, s session.Session) (DropletResult, error)
}

type Destroyer interface {
	ForceDestroySessionServer(ctx context.Context, s session.Session) (DestroyResult, error)
}

type DestroyResult struct {
	DropletID string
}
