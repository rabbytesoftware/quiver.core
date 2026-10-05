package core

import (
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/logger"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

type Core struct {
	metadata *metadata.Metadata
	config   *config.Config
	logs     logring.Ring
}

// New returns Core plus a shutdown func that closes the log file handle.
// The caller owns calling it: a real daemon process only ever calls New
// once and can let the OS reclaim the handle on exit, but anything that
// constructs a Core repeatedly in one process — tests above all — leaks a
// held-open file every time otherwise, which Windows refuses to let a
// later os.RemoveAll (e.g. t.TempDir's cleanup) delete.
func New() (*Core, func() error) {
	cfg := config.Get()
	logs := logring.New()
	shutdown := logger.Init(config.GetLogger(), logs.Wrap)
	logCorrections(config.Corrections())

	return &Core{
		metadata: metadata.Get(),
		config:   cfg,
		logs:     logs,
	}, shutdown
}

// NewAt is New rooted at homeDir instead of the process-level HOME — used
// when the caller was built with an explicit home override (tests, or a dev
// build's checkout-local .quiver), so config and logging never touch the
// real ~/.quiver.
func NewAt(homeDir string) (*Core, func() error) {
	cfg, corrections := config.GetAt(homeDir)
	logs := logring.New()
	shutdown := logger.InitAt(homeDir, cfg.Config.Logger, logs.Wrap)
	logCorrections(corrections)

	return &Core{
		metadata: metadata.Get(),
		config:   cfg,
		logs:     logs,
	}, shutdown
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

// Logs returns the ring that holds every record the process logger has
// written since this Core was built, so the console can replay and follow them.
func (c *Core) Logs() logring.Ring {
	return c.logs
}
