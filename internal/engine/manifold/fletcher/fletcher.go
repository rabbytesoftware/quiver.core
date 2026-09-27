package fletcher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type Fletcher interface {
	Fletch(
		ctx context.Context,
		ns domain.Namespace,
		tag string,
	) (Draft, error)
	Probe(
		ctx context.Context,
		ns domain.Namespace,
		tag string,
		hint Hint,
	) (Draft, error)
}

type fletcher struct {
	lookup hosts.Lookup
	picker picker.Picker
}

func New(
	lookup hosts.Lookup,
	pk picker.Picker,
) Fletcher {
	return &fletcher{
		lookup: hosts.Or(lookup),
		picker: pk,
	}
}

func (f *fletcher) Fletch(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) (Draft, error) {
	host, picks, err := f.prepare(ctx, ns, tag)
	if err != nil {
		return Draft{}, err
	}
	page, err := host.RepoPage(ctx, ns)
	if err != nil {
		return Draft{}, fmt.Errorf("fletcher: repo page %s: %w", ns, err)
	}
	raw, err := fetchReadme(ctx, host, ns, tag)
	if err != nil {
		return Draft{}, fmt.Errorf("fletcher: readme %s: %w", ns, err)
	}
	owner, repo := coordinates(ns)
	return draft(forge.Input{
		Repo:        repo,
		Name:        repo,
		Description: page.Description,
		URL:         repoURL(ns, owner, repo),
		Media:       media.Resolve(ctx, host, ns, tag, page, raw),
		Readme:      readme.Transform(raw, readme.RawBase{Owner: owner, Repo: repo, Ref: tag}),
		Picks:       picks,
	})
}

func (f *fletcher) Probe(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
	hint Hint,
) (Draft, error) {
	_, picks, err := f.prepare(ctx, ns, tag)
	if err != nil {
		return Draft{}, err
	}
	owner, repo := coordinates(ns)
	return draft(forge.Input{
		Repo:        repo,
		Name:        hint.nameOr(repo),
		Description: hint.Description,
		URL:         repoURL(ns, owner, repo),
		Picks:       picks,
	})
}

func (f *fletcher) prepare(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) (hosts.Forge, map[domain.OS]picker.Pick, error) {
	host, err := f.forgeFor(ns)
	if err != nil {
		return nil, nil, err
	}
	picks, err := f.pickAll(ctx, host, ns, tag)
	if err != nil {
		return nil, nil, err
	}
	return host, picks, nil
}

func (f *fletcher) forgeFor(
	ns domain.Namespace,
) (hosts.Forge, error) {
	host, ok := f.lookup(ns)
	if !ok {
		return nil, NotFletchableError{Reason: ReasonHostUnsupported}
	}
	capable, ok := host.(hosts.Forge)
	if !ok {
		return nil, NotFletchableError{Reason: ReasonHostUnsupported}
	}
	return capable, nil
}

func (f *fletcher) pickAll(
	ctx context.Context,
	host hosts.Forge,
	ns domain.Namespace,
	tag string,
) (map[domain.OS]picker.Pick, error) {
	assets, err := host.ReleaseAssets(ctx, ns, tag)
	if errors.Is(err, hosts.ErrReleaseNotFound) {
		return nil, NotFletchableError{Reason: ReasonNoReleaseAssets}
	}
	if err != nil {
		return nil, fmt.Errorf("fletcher: release assets %s: %w", ns, err)
	}
	if len(assets) == 0 {
		return nil, NotFletchableError{Reason: ReasonNoReleaseAssets}
	}
	_, repo := coordinates(ns)
	picks := make(map[domain.OS]picker.Pick)
	for _, platform := range domain.AllOS() {
		if pick, ok := f.usablePick(repo, assets, platform); ok {
			picks[platform] = pick
		}
	}
	if len(picks) == 0 {
		return nil, NotFletchableError{Reason: ReasonNoUsableAsset}
	}
	return picks, nil
}

func (f *fletcher) usablePick(
	repo string,
	assets []hosts.Asset,
	platform domain.OS,
) (picker.Pick, bool) {
	pick, ok := f.picker.Pick(repo, assets, platform)
	if !ok {
		return picker.Pick{}, false
	}
	_, usable := forge.LocalFile(pick)
	return pick, usable
}

func draft(
	in forge.Input,
) (Draft, error) {
	confidence, warnings := assess(in.Picks)
	in.Generator = domain.ArrowGenerator{
		Name:       heuristics,
		Confidence: string(confidence),
		Warnings:   warnings,
	}
	manifest, err := forge.Render(in)
	if err != nil {
		return Draft{}, fmt.Errorf("fletcher: render: %w", err)
	}
	return Draft{
		Manifest: manifest,
		Report: Report{
			Heuristics: heuristics,
			Confidence: confidence,
			Warnings:   warnings,
		},
	}, nil
}

func coordinates(
	ns domain.Namespace,
) (string, string) {
	parts := append(strings.SplitN(string(ns.BareNamespace()), "/", 4), "", "")
	return parts[1], parts[2]
}

func repoURL(
	ns domain.Namespace,
	owner string,
	repo string,
) string {
	return "https://" + ns.Domain() + "/" + owner + "/" + repo
}
