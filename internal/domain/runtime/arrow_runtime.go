package runtime

import "github.com/rabbytesoftware/quiver/internal/domain"

type ArrowRuntime struct {
	Namespace  domain.Namespace
	State      domain.ArrowState
	ActiveRun  *RunRecord
	LastReturn *Return
}
