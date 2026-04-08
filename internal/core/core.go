package core

import (
	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/core/logger"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
)

type Core struct {
	metadata *metadata.Metadata
	config   *config.Config
}

func Init() *Core {
	logger.Init(config.GetWatcher())
	return &Core{
		metadata: metadata.Get(),
		config:   config.Get(),
	}
}

func (c *Core) GetMetadata() *metadata.Metadata {
	return c.metadata
}

func (c *Core) GetConfig() *config.Config {
	return c.config
}
