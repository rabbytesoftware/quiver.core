package selfarrow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// UpdatedBinaryName is the file the self-arrow's update lifecycle leaves in
// the execution workdir. It is the `to:` of the manifest's only fetch step, so
// the manifest at the repository root and the process that hands over to what
// that step downloaded have to agree on it.
const UpdatedBinaryName = "quiver-new"

// arrowCatalog is the subset of the arrow catalog EnsureRegistered needs.
// Both the repository-level arrow.Arrow the app container passes and this
// package's tests satisfy it structurally, so EnsureRegistered depends on
// neither directly.
type arrowCatalog interface {
	Exists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// EnsureRegistered adds quiver.core to its own arrow catalog on first boot at
// a given version, so its own drift can be checked and updated through the
// exact same path as any other arrow. A no-op once already registered, and a
// no-op entirely for an unstamped build (an empty version, or the "dev"
// placeholder cmd/quiver falls back to outside ldflags) — neither is a
// resolvable ref any drift check could compare against.
func EnsureRegistered(
	ctx context.Context,
	arrows arrowCatalog,
	version string,
) error {
	if version == "" || version == "dev" {
		return nil
	}

	self, _ := metadata.GetSelfNamespaces()
	ns := self.WithRef(version)

	exists, err := arrows.Exists(ctx, ns)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if exists {
		return nil
	}

	if err := arrows.Seed(ctx, ns, selfmanifest.Raw()); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return nil
}

// PromoteRunningBinary copies src (the currently running executable's own
// path, i.e. os.Executable()'s result) to the stable self-install path under
// homeDir, so a future launch from scratch — a reboot, or Desktop spawning a
// fresh sidecar — picks up whatever version is currently running rather than
// reverting to whatever was there before.
//
// It returns without copying when src already IS the self path. That case is
// real and routine, not defensive: SidecarManager::spawn over in
// quiver.desktop prefers {quiver_home}/self/quiver over its own bundled seed
// the moment one exists, so every sidecar launch after the first is a process
// running out of exactly this destination. Writing a file that is currently
// being executed fails with ETXTBSY on Linux, and the copy would be a no-op
// anyway — the bytes are already there, they are what is running.
//
// An empty homeDir resolves the self path against the process's own home
// (paths.Self) rather than the empty string literally, matching every other
// path pair in this codebase (paths.Store/StoreAt and its siblings): homeDir
// only ever carries an override, and its zero value means "no override".
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

// sameFile reports whether two paths name the same file on disk. os.SameFile
// rather than a string comparison: the two can differ as text and still be
// one file, through a symlink, a hard link or a bind mount, and every one of
// those still makes the write an ETXTBSY.
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

// RetireStale removes every other quiver.core self-arrow record besides the
// one at the current version. Only quiver.core needs exactly one active
// self-record at a time — unlike an ordinary arrow, where multiple installed
// versions genuinely coexisting is supported and expected, a stale
// quiver.core record left behind after a successful self-update is
// dangerous: BeginUpdate against it would fetch into a workdir that may
// still be the file the current process is executing out of.
//
// The refs come from each view's Versions, NOT from ArrowView.Namespace, and
// that distinction is the whole of this function's correctness. The catalog
// list is grouped by repository: an ArrowView's own Namespace is the BARE
// namespace shared by every installed ref of that arrow, and the ref-carrying
// namespaces live one level down in Versions (see store.toArrowView, which
// fills Namespace from the view model and Versions from its VersionRefs).
// Reading the outer one instead means comparing a namespace that can never
// carry an "@" against a prefix that requires one, so nothing is ever
// matched and nothing is ever retired.
func RetireStale(
	ctx context.Context,
	arrows arrowCatalog,
	version string,
) error {
	if version == "" || version == "dev" {
		return nil
	}

	self, _ := metadata.GetSelfNamespaces()
	current := self.WithRef(version)

	items, err := arrows.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("selfarrow: retire stale: %w", err)
	}

	for _, item := range items {
		for _, ns := range staleRefs(item, self, current) {
			if err := arrows.Remove(ctx, ns); err != nil {
				return fmt.Errorf("selfarrow: retire stale %s: %w", ns, err)
			}
		}
	}
	return nil
}

// staleRefs picks the ref-qualified quiver.core namespaces in one catalog
// view that are not the version currently running.
func staleRefs(
	item models.ArrowView,
	self domain.Namespace,
	current domain.Namespace,
) []domain.Namespace {
	var stale []domain.Namespace
	for _, version := range item.Versions {
		ns := version.Namespace
		if ns == current {
			continue
		}
		if !strings.HasPrefix(ns.String(), string(self)+"@") {
			continue
		}
		stale = append(stale, ns)
	}
	return stale
}
