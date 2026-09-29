package ownership

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

type Holder struct {
	Exists    bool
	Namespace domain.Namespace
	Target    string
}

func (h Holder) Refusal(
	bare domain.Namespace,
) string {
	if !h.Exists || h.Namespace == bare {
		return ""
	}
	if h.Namespace == "" {
		return models.ReasonUnmanaged
	}
	return fmt.Sprintf("%s %s", models.ReasonForeignOwner, h.Namespace)
}

func Unowned(
	_ string,
) Holder {
	return Holder{}
}
