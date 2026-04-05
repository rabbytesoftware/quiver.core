package projections

import (
	"github.com/char2cs/asynx"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func Init(
	axQuiver asynx.Asynx[domain.Quiver],
	catalog quiverstore.QuiverCatalog,
) error {
	if _, err := axQuiver.Subscribe("quiver.added", OnQuiverAdded(catalog)); err != nil {
		return err
	}

	if _, err := axQuiver.Subscribe("quiver.updated", OnQuiverUpdated(catalog)); err != nil {
		return err
	}

	if _, err := axQuiver.Subscribe("quiver.removed", OnQuiverRemoved(catalog)); err != nil {
		return err
	}

	return nil
}
