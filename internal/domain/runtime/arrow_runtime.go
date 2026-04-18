package runtime

import "github.com/rabbytesoftware/quiver/internal/domain"

type ArrowRuntime struct {
	Ref        domain.Namespace
	State      domain.ArrowState
	Execution  *Execution
	LastReturn *Return
}
