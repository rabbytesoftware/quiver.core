package runtime

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ArrowRuntime struct {
	Ref            domain.Namespace
	State          domain.ArrowState
	Execution      *Execution
	LastReturn     *Return
	PendingDepSync *DepSyncInfo
	// PendingActivation is what a finished method staged and left waiting for
	// a daemon restart to take effect.
	PendingActivation *PendingActivation
}

type DepSyncInfo struct {
	AddedDeps   []domain.Namespace
	RemovedDeps []domain.Namespace
}

// PendingActivation is a binary a finished method staged. Size and Digest are
// taken when it was staged, so a file truncated or replaced since is told
// apart from the one that was verified.
type PendingActivation struct {
	Version string
	// Commit tells two builds of a rolling release apart, which share a Version.
	Commit   string
	StagedAt time.Time
	Path     string
	Size     int64
	Digest   string
	// Activating is set just before the daemon hands over, so a handover that
	// failed is told apart at the next boot from a stage nobody applied yet.
	Activating bool
}
