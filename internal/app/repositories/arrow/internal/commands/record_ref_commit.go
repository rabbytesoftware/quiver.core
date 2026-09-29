package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// RecordRefCommit stamps the commit an arrow's ref currently stands at, for a
// ref that keeps its name while its commit moves. quiver.core sends it on boot
// with the commit of the build that is running, which is how an update that
// re-launches the daemon under the same ref still lands as "now current".
type RecordRefCommit struct {
	Namespace    domain.Namespace
	RefCommitSHA string
}

func (c RecordRefCommit) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordRefCommit) EventName() string {
	return "arrow.ref_commit_recorded." + c.Namespace.String()
}

func (c RecordRefCommit) ShouldSnapshot() bool {
	return true
}

func (c RecordRefCommit) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("record ref commit: %w", asynxModels.ErrValidation)
	}
	return nil
}

// EmitEvent clears the drift answer along with the stamp: Outdated and
// RecommendedRef were computed against the previous commit, and RecordVersionCheck
// is their only other writer.
func (c RecordRefCommit) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.RefCommitSHA = c.RefCommitSHA
	next.Outdated = false
	next.RecommendedRef = ""
	return next
}
