package fallback

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/gather"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const inferredFilename = "ARROW.md"

type fallback struct {
	lookup   hosts.Lookup
	releases models.Releases
	drafter  gather.Drafter
}

func New(
	lookup hosts.Lookup,
	releases models.Releases,
	drafter gather.Drafter,
) models.Fletcher {
	return &fallback{
		lookup:   lookup,
		releases: releases,
		drafter:  drafter,
	}
}

func (f *fallback) Recover(
	ctx context.Context,
	ns domain.Namespace,
	cause error,
) ([]byte, string, error) {
	if !errors.Is(cause, resolver.ErrManifestNotFound) || ns.BareNamespace().IsQuiverHosted() {
		return nil, "", cause
	}
	draft, err := f.draftAcrossTags(ctx, ns)
	if err != nil {
		return nil, "", fmt.Errorf("fletcher: fallback %s: %w", ns, fetchFailure(err))
	}
	return draft, inferredFilename, nil
}

func (f *fallback) draftAcrossTags(
	ctx context.Context,
	ns domain.Namespace,
) ([]byte, error) {
	ref := ns.Ref()
	if ref == "" {
		return f.draftFromReleases(ctx, ns, ref)
	}

	draft, err := f.drafter.Draft(ctx, ns, ref)
	if !releaseMissing(err) || !f.isBranch(ctx, ns, ref) {
		return draft, err
	}
	return f.draftFromReleases(ctx, ns, ref)
}

func (f *fallback) draftFromReleases(
	ctx context.Context,
	ns domain.Namespace,
	tried string,
) ([]byte, error) {
	err := error(models.NotFletchableError{Reason: models.ReasonNoReleaseAssets})
	var lookupErr error
	seen := map[string]bool{"": true, tried: true}

	for _, source := range f.releaseSources() {
		tag, sourceErr := source(ctx, ns.BareNamespace())
		lookupErr = errors.Join(lookupErr, sourceErr)
		if seen[tag] {
			continue
		}
		seen[tag] = true

		draft, runErr := f.drafter.Draft(ctx, ns, tag)
		if !releaseMissing(runErr) {
			return draft, runErr
		}
		err = runErr
	}

	if lookupErr != nil {
		return nil, lookupErr
	}
	return nil, err
}

func (f *fallback) releaseSources() []func(context.Context, domain.Namespace) (string, error) {
	return []func(context.Context, domain.Namespace) (string, error){
		f.latestStable,
		f.latestUnstable,
	}
}

func (f *fallback) isBranch(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
) bool {
	if host, ok := f.lookup(ns); ok && slices.Contains(host.DefaultBranches(), ref) {
		return true
	}
	branch, _, err := f.releases.ResolveDefaultBranch(ctx, ns.BareNamespace())
	return err == nil && branch == ref
}

func fetchFailure(
	err error,
) error {
	switch {
	case errors.Is(err, models.ErrNotFletchable):
		return fmt.Errorf("%w: %w", resolver.ErrManifestNotFound, err)
	case errors.Is(err, resolver.ErrFetchFailed):
		return err
	default:
		return fmt.Errorf("%w: %w", resolver.ErrFetchFailed, err)
	}
}

func releaseMissing(
	err error,
) bool {
	var nf models.NotFletchableError
	return errors.As(err, &nf) && nf.Reason == models.ReasonNoReleaseAssets
}
