package runtimeinternal

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Exposer interface {
	Apply(
		ctx context.Context,
		ns domain.Namespace,
		workdir string,
	) *domainRuntime.ExposeResult
	Reapply(
		ctx context.Context,
		ns domain.Namespace,
	) *domainRuntime.ExposeResult
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	)
}
