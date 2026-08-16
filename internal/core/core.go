package core

import (
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/logger"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

type Core struct {
	metadata *metadata.Metadata
	config   *config.Config
}

func New() *Core {
	_ = logger.Init(config.GetLogger()) // shutdown func: process-lifetime logger, OS closes file handle on exit

	// Reported here rather than during the load: the logger is configured from
	// the very config being loaded, so a warning raised inside Get would go to
	// stderr and never reach the log file.
	for _, fe := range config.Corrections() {
		slog.Warn("config: invalid value, using default", "key", fe.Key, "reason", fe.Message)
	}

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
