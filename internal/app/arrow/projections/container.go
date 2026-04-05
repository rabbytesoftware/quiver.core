package projections

import (
	"github.com/char2cs/asynx"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

func Init(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	catalog arrowstore.ArrowCatalog,
	wiz wizard.Wizard,
) error {
	if _, err := axArrow.Subscribe("arrow.added", OnArrowAdded(catalog)); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe("arrow.updated", OnArrowUpdated(catalog)); err != nil {
		return err
	}

	if _, err := axArrow.Subscribe("arrow.removed", OnArrowRemoved(catalog)); err != nil {
		return err
	}

	if _, err := axRuntime.Subscribe("runtime.mark_stopping", StopCoordinator(wiz)); err != nil {
		return err
	}

	return nil
}
