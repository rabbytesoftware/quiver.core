package manifold

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const inferredFilename = "ARROW.md"

func (m *manifold) ProbeArrow(
	ctx context.Context,
	ns domain.Namespace,
	hint fletcher.Hint,
) (*domain.Arrow, []byte, error) {
	if m.fl == nil {
		return nil, nil, fmt.Errorf("manifold: probe %s: %w", ns, fletcher.NotFletchableError{Reason: fletcher.ReasonDisabled})
	}
	if ns.BareNamespace().IsQuiverHosted() {
		return nil, nil, fmt.Errorf("manifold: probe %s: %w", ns, fletcher.NotFletchableError{Reason: fletcher.ReasonHostUnsupported})
	}

	draft, tag, err := m.draftAcrossTags(ctx, ns, func(tag string) (fletcher.Draft, error) {
		return m.fl.Probe(ctx, ns, tag, hint)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("manifold: probe %s: %w", ns, fetchFailure(err))
	}

	arrow, err := m.ParseArrow(draft.Manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("manifold: probe %s: %w", ns, err)
	}
	arrow.Namespace = ns.BareNamespace().WithRef(tag)
	return arrow, draft.Manifest, nil
}

func (m *manifold) shouldFletch(
	ns domain.Namespace,
	err error,
) bool {
	return m.fl != nil &&
		errors.Is(err, resolver.ErrManifestNotFound) &&
		!ns.BareNamespace().IsQuiverHosted()
}

func (m *manifold) fletchArrow(
	ctx context.Context,
	ns domain.Namespace,
) ([]byte, string, error) {
	draft, _, err := m.draftAcrossTags(ctx, ns, func(tag string) (fletcher.Draft, error) {
		return m.fl.Fletch(ctx, ns, tag)
	})
	if err != nil {
		return nil, "", fmt.Errorf("manifold: fletch %s: %w", ns, fetchFailure(err))
	}
	return draft.Manifest, inferredFilename, nil
}

func (m *manifold) draftAcrossTags(
	ctx context.Context,
	ns domain.Namespace,
	run func(tag string) (fletcher.Draft, error),
) (fletcher.Draft, string, error) {
	ref := ns.Ref()
	if ref == "" {
		return m.draftFromReleases(ctx, ns, run, ref)
	}

	draft, err := run(ref)
	if !releaseMissing(err) || !m.isBranch(ctx, ns, ref) {
		return draft, ref, err
	}
	return m.draftFromReleases(ctx, ns, run, ref)
}

func (m *manifold) draftFromReleases(
	ctx context.Context,
	ns domain.Namespace,
	run func(tag string) (fletcher.Draft, error),
	tried string,
) (fletcher.Draft, string, error) {
	err := error(fletcher.NotFletchableError{Reason: fletcher.ReasonNoReleaseAssets})
	var lookupErr error
	seen := map[string]bool{"": true, tried: true}

	for _, source := range m.releaseSources() {
		tag, sourceErr := source(ctx, ns)
		lookupErr = errors.Join(lookupErr, lookupFailure(sourceErr))
		if seen[tag] {
			continue
		}
		seen[tag] = true

		draft, runErr := run(tag)
		if !releaseMissing(runErr) {
			return draft, tag, runErr
		}
		err = runErr
	}

	if lookupErr != nil {
		return fletcher.Draft{}, "", lookupErr
	}
	return fletcher.Draft{}, "", err
}

func fetchFailure(
	err error,
) error {
	if errors.Is(err, fletcher.ErrNotFletchable) || errors.Is(err, resolver.ErrFetchFailed) {
		return err
	}
	return fmt.Errorf("%w: %w", resolver.ErrFetchFailed, err)
}

func lookupFailure(
	err error,
) error {
	if errors.Is(err, ErrNoLatestStable) || errors.Is(err, ErrNoTagInChannel) {
		return nil
	}
	return err
}

func (m *manifold) releaseSources() []func(context.Context, domain.Namespace) (string, error) {
	return []func(context.Context, domain.Namespace) (string, error){
		m.latestStableTag,
		m.firstChannelTag,
	}
}

func (m *manifold) isBranch(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
) bool {
	if host, ok := m.hosts(ns); ok && slices.Contains(host.DefaultBranches(), ref) {
		return true
	}
	branch, _, err := m.ResolveDefaultBranch(ctx, ns.BareNamespace())
	return err == nil && branch == ref
}

func (m *manifold) latestStableTag(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	tag, err := m.ResolveLatestStable(ctx, ns.BareNamespace())
	if errors.Is(err, ErrNoLatestStable) {
		return "", nil
	}
	return tag, err
}

func (m *manifold) firstChannelTag(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	channels, err := m.ListChannels(ctx, ns.BareNamespace())
	if err != nil {
		return "", err
	}

	for _, channel := range channels {
		if channel.Name == StableChannel || channel.IsDefaultBranchFallback {
			continue
		}
		return channel.Latest, nil
	}
	return "", nil
}

func releaseMissing(
	err error,
) bool {
	var nf fletcher.NotFletchableError
	return errors.As(err, &nf) && nf.Reason == fletcher.ReasonNoReleaseAssets
}
