package selfarrow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// UpdatedBinaryName is the name the self-arrow's update fetch step downloads
// to, and the binary handover reads back.
const UpdatedBinaryName = "quiver-new"

// arrowCatalog is the subset of the arrow catalog EnsureRegistered needs.
type arrowCatalog interface {
	Adopt(
		ctx context.Context,
		ns domain.Namespace,
		kind domain.SelectorKind,
		resolved domain.Resolved,
		manifest []byte,
		filename string,
	) error
	CheckVersionNow(
		ctx context.Context,
		ns domain.Namespace,
	)
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// runtimeMarker is the subset of the runtime repository EnsureRegistered
// needs to settle its own row at ready.
type runtimeMarker interface {
	GetState(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.ArrowState, error)
	ClearVersionBadge(
		ctx context.Context,
		ns domain.Namespace,
	) error
	MarkReady(
		ctx context.Context,
		ns domain.Namespace,
		lastReturn *domainRuntime.Return,
	) error
}

// Channel returns the channel quiver.core's own row tracks: an explicit
// self_update_channel wins over the channel the build was published under.
func Channel(
	configured string,
	built string,
) string {
	if configured != "" {
		return configured
	}
	return built
}

// EnsureRegistered adopts the running build as quiver.core's own catalog row,
// offline, from the embedded manifest: quiver.core@<channel> tracking that
// channel, or quiver.core@<version> pinned to it when no channel is known (a
// build that declares none). The identity survives updates, so the boot after
// one only moves the row onto the new state. Rows earlier builds filed under
// any other identity are removed, and the row's runtime is settled at ready:
// reaching this function running is proof of that. A no-op for an unstamped
// build (empty version, or "dev"), since neither is a resolvable ref.
func EnsureRegistered(
	ctx context.Context,
	arrows arrowCatalog,
	rt runtimeMarker,
	version string,
	commit string,
	channel string,
) error {
	if version == "" || version == "dev" {
		return nil
	}

	self, _ := metadata.GetSelfNamespaces()
	ns, kind := self.WithRef(version), domain.SelectorPin
	if channel != "" {
		ns, kind = self.WithRef(channel), domain.SelectorChannel
	}
	resolved := domain.Resolved{Ref: version, Commit: commit, Fingerprint: commit}

	if err := arrows.Adopt(ctx, ns, kind, resolved, selfmanifest.Raw(), "ARROW.md"); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if err := settleRuntime(ctx, arrows, rt, ns); err != nil {
		return err
	}
	arrows.CheckVersionNow(ctx, ns)
	return removeOtherSelfRows(ctx, arrows, self, ns)
}

// settleRuntime lands ns's runtime at Ready. An absent runtime is marked
// ready; ready -> ready is not a valid transition, so a ready one is left
// alone. An Outdated one is the badge the drift check set before this build's
// own update, whose commit is skipped for the core's row: once Adopt has
// moved the row onto this build and nothing newer is available, the badge is
// cleared offline rather than waiting for the next version check. Any other
// state is left alone.
func settleRuntime(
	ctx context.Context,
	arrows arrowCatalog,
	rt runtimeMarker,
	ns domain.Namespace,
) error {
	state, err := rt.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if state == domain.ArrowStateAbsent {
		if err := rt.MarkReady(ctx, ns, nil); err != nil {
			return fmt.Errorf("selfarrow: ensure registered: %w", err)
		}
		return nil
	}
	if state != domain.ArrowStateOutdated {
		return nil
	}

	available, err := availableOf(ctx, arrows, ns)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if available != nil {
		return nil
	}
	if err := rt.ClearVersionBadge(ctx, ns); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return nil
}

// availableOf reads what the catalog records as available ahead of ns; nil
// when nothing is, or when ns is not listed.
func availableOf(
	ctx context.Context,
	arrows arrowCatalog,
	ns domain.Namespace,
) (*domain.Available, error) {
	views, err := arrows.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, view := range views {
		for _, version := range view.Versions {
			if version.Namespace == ns {
				return version.Metadata.Available, nil
			}
		}
	}
	return nil, nil
}

// removeOtherSelfRows removes every quiver.core row but current, through the
// catalog's own forget cascade. A row that cannot be removed does not stop
// the others from going.
func removeOtherSelfRows(
	ctx context.Context,
	arrows arrowCatalog,
	self domain.Namespace,
	current domain.Namespace,
) error {
	views, err := arrows.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("selfarrow: remove other self rows: %w", err)
	}

	var errs []error
	for _, view := range views {
		for _, version := range view.Versions {
			if version.Namespace.BareNamespace() != self || version.Namespace == current {
				continue
			}
			err := arrows.Remove(ctx, version.Namespace)
			if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
				errs = append(errs, fmt.Errorf("selfarrow: remove %s: %w", version.Namespace, err))
			}
		}
	}
	return errors.Join(errs...)
}

// PromoteRunningBinary copies src (the running executable's own path) to the
// stable self-install path under homeDir, so a fresh launch -- a reboot, or
// Desktop spawning a sidecar -- picks up the version currently running.
//
// A no-op when src already is the self path: quiver.desktop's sidecar prefers
// that path over its bundled seed once one exists, and writing a file that is
// currently executing fails with ETXTBSY anyway.
func PromoteRunningBinary(
	src string,
	homeDir string,
) error {
	var selfDir string
	var err error
	if homeDir != "" {
		selfDir, err = paths.SelfAt(homeDir)
	} else {
		selfDir, err = paths.Self()
	}
	if err != nil {
		return fmt.Errorf("selfarrow: promote: %w", err)
	}

	dst := filepath.Join(selfDir, binaryName())
	if sameFile(src, dst) {
		return nil
	}

	data, err := os.ReadFile(src) // #nosec -- src is os.Executable()'s own result, passed in by the caller, never external input
	if err != nil {
		return fmt.Errorf("selfarrow: promote: read %s: %w", src, err)
	}

	if err := os.WriteFile(dst, data, 0o755); err != nil { // #nosec -- the promoted file is an executable quiver binary; it must carry the executable bit
		return fmt.Errorf("selfarrow: promote: write %s: %w", dst, err)
	}
	return nil
}

// sameFile reports whether two paths name the same file on disk -- a
// symlink, hard link or bind mount can differ as text yet still be one file.
func sameFile(a, b string) bool {
	aInfo, err := os.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "quiver.exe"
	}
	return "quiver"
}
