package engine

import (
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

// NewProviders exposes the unexported newProviders for white-box tests.
func NewProviders(
	platforms metadata.Platforms,
	search config.Search,
) ([]provider.Provider, error) {
	return newProviders(platforms, search)
}
