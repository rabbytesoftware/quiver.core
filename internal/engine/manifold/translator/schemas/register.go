package schemas

import (
	arrowv0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas/arrow/v0"
	quiverv0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas/quiver/v0"
)

// register adds all known schema mappers to the registry.
//
// Schema versions are registered explicitly rather than via auto-discovery.
// To add a new schema version (e.g., arrow@v1), import the new mapper
// package and add it alongside the existing registrations here.
func (r *Registry) register() {
	r.arrowMappers["arrow@v0"] = arrowv0.NewMapper()
	r.quiverMappers["quiver@v0"] = quiverv0.NewMapper()
}
