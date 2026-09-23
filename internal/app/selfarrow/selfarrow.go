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
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// UpdatedBinaryName is the name the self-arrow's update fetch step downloads
// to, and the binary handover reads back.
const UpdatedBinaryName = "quiver-new"

// arrowCatalog is the subset of the arrow catalog EnsureRegistered needs.
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
	// UpgradeVersionSeeded moves the self-arrow's row from oldNs to newNs
	// using the manifest bytes already embedded in this binary.
	UpgradeVersionSeeded(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
		data []byte,
	) error
	SetChannel(
		ctx context.Context,
		ns domain.Namespace,
		channel string,
		ref string,
	) error
}

// runtimeMarker is the subset of the runtime repository EnsureRegistered
// needs to mark its own row ready.
type runtimeMarker interface {
	GetState(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.ArrowState, error)
	MarkReady(
		ctx context.Context,
		ns domain.Namespace,
		lastReturn *domainRuntime.Return,
	) error
}

// EnsureRegistered lands quiver.core's own catalog row on the version
// currently running: a first boot seeds it, every boot after an update moves
// the existing row onto the new ref instead of leaving the old one behind.
// Every successful path -- including a steady-state boot already registered
// at this version -- (re)stamps the configured channel and marks the row's
// runtime ready, so a channel set after the daemon last changed version
// still takes effect on restart, and the self-arrow never gets stuck
// reporting "absent": reaching this function running IS proof it is ready,
// unlike quiver.desktop, which needs an external preinstalled probe to
// reach the same conclusion. A no-op altogether for an unstamped build
// (empty version, or "dev"), since neither is a resolvable ref.
func EnsureRegistered(
	ctx context.Context,
	arrows arrowCatalog,
	rt runtimeMarker,
	version string,
	channel string,
) error {
	if version == "" || version == "dev" {
		return nil
	}

	self, _ := metadata.GetSelfNamespaces()
	newNs := self.WithRef(version)

	exists, err := arrows.Exists(ctx, newNs)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if exists {
		return finishRegistration(ctx, arrows, rt, newNs, channel)
	}

	oldNs, found, err := currentSelfRow(ctx, arrows, self)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if !found {
		if err := arrows.Seed(ctx, newNs, selfmanifest.Raw()); err != nil {
			return fmt.Errorf("selfarrow: ensure registered: %w", err)
		}
		return finishRegistration(ctx, arrows, rt, newNs, channel)
	}

	if err := arrows.UpgradeVersionSeeded(ctx, oldNs, newNs, selfmanifest.Raw()); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return finishRegistration(ctx, arrows, rt, newNs, channel)
}

// finishRegistration applies every follow-up a freshly seeded, moved, or
// already-registered row needs: the configured channel, and marking the
// runtime ready. Called on all three of EnsureRegistered's exit paths, since
// every one of them needs the same follow-up, not just the "new
// registration" ones.
func finishRegistration(
	ctx context.Context,
	arrows arrowCatalog,
	rt runtimeMarker,
	ns domain.Namespace,
	channel string,
) error {
	if err := stampConfiguredChannel(ctx, arrows, ns, channel); err != nil {
		return err
	}
	return markReadyIfAbsent(ctx, rt, ns)
}

// stampConfiguredChannel applies channel to newNs's freshly seeded or moved
// row, when one was configured. A no-op for an empty channel: "no
// preference" leaves the row exactly as Seed/UpgradeVersionSeeded already
// left it, unchanged from before this parameter existed. Always passes an
// empty ref to SetChannel: self-registration re-stamps a configured channel
// on every boot, never a specific pinned version within it.
func stampConfiguredChannel(
	ctx context.Context,
	arrows arrowCatalog,
	ns domain.Namespace,
	channel string,
) error {
	if channel == "" {
		return nil
	}
	if err := arrows.SetChannel(ctx, ns, channel, ""); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return nil
}

// markReadyIfAbsent lands ns's runtime aggregate at Ready when it is
// currently absent -- the state right after a fresh Seed/UpgradeVersionSeeded,
// or a steady-state boot that somehow never got marked. absent -> ready is a
// valid domain.ArrowState transition, but ready -> ready is not (see this
// repo's CLAUDE.md §3.2), and EnsureRegistered runs on every boot, so this
// checks first rather than calling MarkReady unconditionally, which would
// otherwise raise a harmless-but-noisy validation error on every boot after
// the first. GetState reports ArrowStateAbsent both when the aggregate
// genuinely holds that state and when it does not exist yet, so no special
// case is needed for the very first self-registration.
func markReadyIfAbsent(
	ctx context.Context,
	rt runtimeMarker,
	ns domain.Namespace,
) error {
	state, err := rt.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if state != domain.ArrowStateAbsent {
		return nil
	}
	if err := rt.MarkReady(ctx, ns, nil); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return nil
}

// currentSelfRow finds quiver.core's one self-arrow record, if any -- there
// is never more than one, since EnsureRegistered always moves the existing
// row rather than adding a second.
//
// Refs come from each view's Versions, not ArrowView.Namespace: the outer
// Namespace is the bare repository shared by every installed ref, so
// comparing it against a "@"-suffixed prefix would never match.
func currentSelfRow(
	ctx context.Context,
	arrows arrowCatalog,
	self domain.Namespace,
) (domain.Namespace, bool, error) {
	items, err := arrows.List(ctx, nil)
	if err != nil {
		return "", false, err
	}

	prefix := string(self) + "@"
	for _, item := range items {
		for _, version := range item.Versions {
			if strings.HasPrefix(version.Namespace.String(), prefix) {
				return version.Namespace, true, nil
			}
		}
	}
	return "", false, nil
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
