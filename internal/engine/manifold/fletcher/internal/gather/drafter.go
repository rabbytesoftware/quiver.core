package gather

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/confidence"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type Drafter interface {
	Draft(
		ctx context.Context,
		ns domain.Namespace,
		tag string,
	) ([]byte, error)
}

const (
	heuristics    = "fletcher/1"
	standInDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

type drafter struct {
	lookup hosts.Lookup
	picker picker.Picker
	fetch  fetcher
}

func New(
	lookup hosts.Lookup,
	pk picker.Picker,
	timeout time.Duration,
) Drafter {
	return &drafter{
		lookup: lookup,
		picker: pk,
		fetch:  fetcher{timeout: timeout},
	}
}

func (d *drafter) Draft(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) ([]byte, error) {
	host, ok := d.lookup(ns)
	if !ok {
		return nil, models.NotFletchableError{Reason: models.ReasonHostUnsupported}
	}
	src, err := d.gather(ctx, host, ns, tag)
	if err != nil {
		return nil, err
	}
	_, repo := coordinates(ns)
	return draft(forge.Input{
		Repo:        repo,
		Name:        repo,
		Description: src.page.description,
		URL:         host.RepoPageURL(ns),
		Media:       media.Resolve(ctx, d.fetch.prefixOf(maxProbeBytes), host, ns, tag, src.page.socialImage, src.readme, src.icon),
		Readme:      readme.Transform(src.readme, readmeBase(host, ns, tag)),
		Picks:       src.picks,
	})
}

func readmeBase(
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) readme.RawBase {
	raw, _ := host.RawFileURL(ns, ref, readme.FilePlaceholder)
	blob, _ := host.BlobFileURL(ns, ref, readme.FilePlaceholder)
	return readme.RawBase{Raw: raw, Blob: blob}
}

func (d *drafter) pickAll(
	ctx context.Context,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
) (map[domain.OS]picker.Pick, error) {
	assets, err := host.ReleaseAssets(ctx, ns, tag)
	if err != nil {
		return nil, fmt.Errorf("fletcher: release assets %s: %w", ns, err)
	}
	if len(assets) == 0 {
		return nil, models.NotFletchableError{Reason: models.ReasonNoReleaseAssets}
	}
	_, repo := coordinates(ns)
	picks := make(map[domain.OS]picker.Pick)
	for _, platform := range domain.AllOS() {
		if pick, ok := d.usablePick(repo, assets, platform); ok {
			picks[platform] = pick
		}
	}
	if len(picks) > 0 {
		return picks, nil
	}
	if d.digestWasTheBlocker(repo, assets) {
		return nil, models.NotFletchableError{Reason: models.ReasonNoDigest}
	}
	return nil, models.NotFletchableError{Reason: models.ReasonNoUsableAsset}
}

func (d *drafter) digestWasTheBlocker(
	repo string,
	assets []domain.ReleaseAsset,
) bool {
	digested := make([]domain.ReleaseAsset, len(assets))
	missing := false
	for i, a := range assets {
		if a.Digest == "" {
			a.Digest = standInDigest
			missing = true
		}
		digested[i] = a
	}
	if !missing {
		return false
	}
	for _, platform := range domain.AllOS() {
		if _, ok := d.usablePick(repo, digested, platform); ok {
			return true
		}
	}
	return false
}

func (d *drafter) usablePick(
	repo string,
	assets []domain.ReleaseAsset,
	platform domain.OS,
) (picker.Pick, bool) {
	pick, ok := d.picker.Pick(repo, assets, platform)
	if !ok {
		return picker.Pick{}, false
	}
	_, usable := forge.LocalFile(pick)
	return pick, usable
}

func draft(
	in forge.Input,
) ([]byte, error) {
	level, warnings := confidence.Assess(in.Picks)
	if level == confidence.ConfidenceLow {
		return nil, models.NotFletchableError{Reason: models.ReasonLowConfidence}
	}
	in.Generator = domain.ArrowGenerator{
		Name:       heuristics,
		Confidence: string(level),
		Warnings:   warnings,
	}
	manifest, err := forge.Render(in)
	if err != nil {
		return nil, fmt.Errorf("fletcher: render: %w", err)
	}
	return manifest, nil
}

func coordinates(
	ns domain.Namespace,
) (string, string) {
	parts := append(strings.SplitN(string(ns.BareNamespace()), "/", 4), "", "")
	return parts[1], parts[2]
}
