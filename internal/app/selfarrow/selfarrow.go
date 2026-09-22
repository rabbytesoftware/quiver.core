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
	) error
}

// EnsureRegistered lands quiver.core's own catalog row on the version
// currently running: a first boot seeds it, every boot after an update moves
// the existing row onto the new ref instead of leaving the old one behind.
// Every successful path -- including a steady-state boot already registered
// at this version -- (re)stamps the configured channel, so a channel set
// after the daemon last changed version still takes effect on restart. A
// no-op altogether for an unstamped build (empty version, or "dev"), since
// neither is a resolvable ref.
func EnsureRegistered(
	ctx context.Context,
	arrows arrowCatalog,
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
		return stampConfiguredChannel(ctx, arrows, newNs, channel)
	}

	oldNs, found, err := currentSelfRow(ctx, arrows, self)
	if err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	if !found {
		if err := arrows.Seed(ctx, newNs, selfmanifest.Raw()); err != nil {
			return fmt.Errorf("selfarrow: ensure registered: %w", err)
		}
		return stampConfiguredChannel(ctx, arrows, newNs, channel)
	}

	if err := arrows.UpgradeVersionSeeded(ctx, oldNs, newNs, selfmanifest.Raw()); err != nil {
		return fmt.Errorf("selfarrow: ensure registered: %w", err)
	}
	return stampConfiguredChannel(ctx, arrows, newNs, channel)
}

// stampConfiguredChannel applies channel to newNs's freshly seeded or moved
// row, when one was configured. A no-op for an empty channel: "no
// preference" leaves the row exactly as Seed/UpgradeVersionSeeded already
// left it, unchanged from before this parameter existed.
func stampConfiguredChannel(
	ctx context.Context,
	arrows arrowCatalog,
	ns domain.Namespace,
	channel string,
) error {
	if channel == "" {
		return nil
	}
	if err := arrows.SetChannel(ctx, ns, channel); err != nil {
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
