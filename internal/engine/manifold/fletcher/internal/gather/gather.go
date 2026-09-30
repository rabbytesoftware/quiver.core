package gather

import (
	"context"
	"fmt"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type sources struct {
	picks     map[domain.OS]picker.Pick
	picksErr  error
	page      repoPage
	pageErr   error
	readme    []byte
	readmeErr error
	icon      string
	avatar    string
	meta      domain.RepoMetadata
}

func (d *drafter) gather(
	ctx context.Context,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
) (sources, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var src sources
	var wg sync.WaitGroup
	wg.Add(6)
	go func() {
		defer wg.Done()
		src.picks, src.picksErr = d.pickAll(ctx, host, ns, tag)
		if src.picksErr != nil {
			cancel()
		}
	}()
	go func() {
		defer wg.Done()
		src.page, src.pageErr = d.repoPage(ctx, host, ns)
	}()
	go func() {
		defer wg.Done()
		src.readme, src.readmeErr = fetchReadme(ctx, d.fetch.document, host, ns, tag)
	}()
	go func() {
		defer wg.Done()
		src.icon = media.ProbeIcon(ctx, d.fetch.prefixOf(maxProbeBytes), host, ns, tag)
	}()
	go func() {
		defer wg.Done()
		src.avatar = media.ProbeAvatar(ctx, d.fetch.prefixOf(maxProbeBytes), host, ns)
	}()
	go func() {
		defer wg.Done()
		src.meta = repoMetadataOf(ctx, host, ns)
	}()
	wg.Wait()

	return src, src.firstError(ns)
}

func (s sources) firstError(
	ns domain.Namespace,
) error {
	if s.picksErr != nil {
		return s.picksErr
	}
	if s.pageErr != nil {
		return fmt.Errorf("fletcher: repo page %s: %w", ns, s.pageErr)
	}
	if s.readmeErr != nil {
		return fmt.Errorf("fletcher: readme %s: %w", ns, s.readmeErr)
	}
	return nil
}
