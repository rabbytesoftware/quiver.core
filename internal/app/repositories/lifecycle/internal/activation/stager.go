// Package activation holds what the lifecycle does with a method whose
// success only takes effect once the daemon restarts: staging what it
// produced, and applying it on request.
package activation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// Recorder is what staging needs from the runtime repository.
type Recorder interface {
	RecordPendingActivation(
		ctx context.Context,
		ns domain.Namespace,
		pending domainRuntime.PendingActivation,
	) error
}

// Stager records the binary a finished update left in its workdir as pending
// activation.
type Stager interface {
	// Stage fingerprints the artifact the ended update downloaded and records
	// it against the row. target is what the update was moving to; it may be
	// the zero value when none was remembered.
	Stage(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
		target domain.Available,
	) error
}

type stager struct {
	recorder Recorder
	artifact string
	now      func() time.Time
}

// StagerOption configures NewStager.
type StagerOption func(*stager)

// WithClock replaces the clock stamping StagedAt.
func WithClock(now func() time.Time) StagerOption {
	return func(s *stager) { s.now = now }
}

// NewStager builds a Stager for updates whose artifact is the file named
// artifact in the run's workdir.
func NewStager(
	recorder Recorder,
	artifact string,
	opts ...StagerOption,
) Stager {
	s := &stager{recorder: recorder, artifact: artifact, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *stager) Stage(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
	target domain.Available,
) error {
	if rt.LastReturn == nil {
		return errors.New("stage activation: the update left no return")
	}
	workdir := rt.LastReturn.Variables[domain.VarWorkdir]
	if workdir == "" {
		return errors.New("stage activation: the update ran in no workdir")
	}
	version := target.Ref
	if version == "" {
		version = rt.LastReturn.Variables[domain.VarRef]
	}
	if version == "" {
		return errors.New("stage activation: the update names no version")
	}

	path := filepath.Join(workdir, s.artifact)
	size, digest, err := selfupdate.Fingerprint(ctx, path)
	if err != nil {
		return fmt.Errorf("stage activation: %w", err)
	}
	if size == 0 {
		return fmt.Errorf("stage activation: %s is empty", path)
	}

	pending := domainRuntime.PendingActivation{
		Version:  version,
		Commit:   target.Commit,
		StagedAt: s.now().UTC(),
		Path:     path,
		Size:     size,
		Digest:   digest,
	}
	if err := s.recorder.RecordPendingActivation(ctx, rt.Ref, pending); err != nil {
		return fmt.Errorf("stage activation: %w", err)
	}
	return nil
}
