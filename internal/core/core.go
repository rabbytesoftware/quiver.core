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
	cfg := config.Get()
	_ = logger.Init(config.GetLogger()) // shutdown func: process-lifetime logger, OS closes file handle on exit
	logCorrections(config.Corrections())

	return &Core{
		metadata: metadata.Get(),
		config:   cfg,
	}
}

// NewAt is New rooted at homeDir instead of the process-level HOME — used
// when the caller was built with an explicit home override (tests, or a dev
// build's checkout-local .quiver), so config and logging never touch the
// real ~/.quiver.
func NewAt(homeDir string) *Core {
	cfg, corrections := config.GetAt(homeDir)
	_ = logger.InitAt(homeDir, cfg.Config.Logger) // shutdown func: process-lifetime logger, OS closes file handle on exit
	logCorrections(corrections)

	return &Core{
		metadata: metadata.Get(),
		config:   cfg,
	}
}

// logCorrections reports the fields a config load replaced with their
// defaults. Reported here rather than during the load: the logger is
// configured from the very config being loaded, so a warning raised inside
// Get/GetAt would go to stderr and never reach the log file.
func logCorrections(corrections []config.FieldError) {
	for _, fe := range corrections {
		slog.Warn("config: invalid value, using default", "key", fe.Key, "reason", fe.Message)
	}
}

func (c *Core) GetMetadata() *metadata.Metadata {
	return c.metadata
}

func (c *Core) GetConfig() *config.Config {
	return c.config
}
