package adapter

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

// Container holds all adapter-layer event and snapshot stores, paired per aggregate.
type Container struct {
	Arrow   Stores
	Runtime Stores
	Quiver  Stores
	closers []io.Closer
}

// Close closes all event and snapshot store database connections, checkpointing
// WAL files and releasing file handles. Must be called during shutdown before
// temp directories or process-level cleanup runs.
func (c *Container) Close() error {
	var errs []error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type adapterOpts struct{ homeDir string }

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var.
func WithHomeDir(dir string) Option {
	return func(o *adapterOpts) { o.homeDir = dir }
}

// New constructs all adapter event and snapshot stores.
func New(opts ...Option) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	eventsPath, err := resolveEventsPath(cfg.homeDir)
	if err != nil {
		return nil, fmt.Errorf("adapter: %w", err)
	}

	var closers []io.Closer

	arrow, arrowClosers, err := openStores(eventsPath, "arrow", "arrow.db", "arrow_snapshots.db")
	if err != nil {
		return nil, err
	}
	closers = append(closers, arrowClosers...)

	runtime, runtimeClosers, err := openStores(eventsPath, "runtime", "runtime.db", "runtime_snapshots.db")
	if err != nil {
		closeAll(closers)
		return nil, err
	}
	closers = append(closers, runtimeClosers...)

	quiver, quiverClosers, err := openStores(eventsPath, "quiver", "collection.db", "collection_snapshots.db")
	if err != nil {
		closeAll(closers)
		return nil, err
	}
	closers = append(closers, quiverClosers...)

	return &Container{
		Arrow:   arrow,
		Runtime: runtime,
		Quiver:  quiver,
		closers: closers,
	}, nil
}

// resolveEventsPath returns the event-store directory, rooted at homeDir when
// set or the process-level HOME otherwise.
func resolveEventsPath(homeDir string) (string, error) {
	if homeDir != "" {
		return paths.EventsAt(homeDir)
	}
	return paths.Events()
}

// openStores opens the paired event and snapshot database files for one
// aggregate. The two files are opened separately by design: the event store
// is single-connection write-pinned, and asynx's reader issues
// SnapshotStore.Get first on every read, so sharing a file would serialize
// that read behind any in-flight Append.
func openStores(
	eventsPath string,
	name string,
	eventFile string,
	snapshotFile string,
) (Stores, []io.Closer, error) {
	es, err := sqlite.NewEventStore(filepath.Join(eventsPath, eventFile))
	if err != nil {
		return Stores{}, nil, fmt.Errorf("adapter: %s event store: %w", name, err)
	}

	ss, err := sqlite.NewSnapshotStore(filepath.Join(eventsPath, snapshotFile))
	if err != nil {
		_ = es.Close()
		return Stores{}, nil, fmt.Errorf("adapter: %s snapshot store: %w", name, err)
	}

	return Stores{Events: es, Snapshots: ss}, []io.Closer{es, ss}, nil
}

// closeAll closes every closer, best-effort. Used to release handles already
// opened when a later store in the same New call fails to open.
func closeAll(closers []io.Closer) {
	for _, cl := range closers {
		_ = cl.Close()
	}
}
