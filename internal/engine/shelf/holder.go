package shelf

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type holder struct {
	exists    bool
	namespace domain.Namespace
}

func (h holder) refusal(
	bare domain.Namespace,
) string {
	if !h.exists || h.namespace == bare {
		return ""
	}
	if h.namespace == "" {
		return reasonUnmanaged
	}
	return fmt.Sprintf("%s %s", reasonForeignOwner, h.namespace)
}
