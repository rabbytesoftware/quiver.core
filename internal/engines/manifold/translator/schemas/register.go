package schemas

import (
	arrowv1 "github.com/rabbytesoftware/quiver/internal/engines/manifold/translator/schemas/arrow/v1"
	quiverv1 "github.com/rabbytesoftware/quiver/internal/engines/manifold/translator/schemas/quiver/v1"
)

func (r *Registry) register() {
	r.arrowMappers["arrow@v1"] = arrowv1.NewMapper()
	r.quiverMappers["quiver@v1"] = quiverv1.NewMapper()
}
