package media

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

// ProbeAvatar checks that the host's stable owner-avatar address serves an
// image and returns that address, never the redirect target it may resolve
// to. It returns "" when the host has none or it does not serve an image.
func ProbeAvatar(
	ctx context.Context,
	fetch Fetch,
	host hosts.Host,
	ns domain.Namespace,
) string {
	url := host.OwnerAvatarURL(ns)
	if url == "" {
		return ""
	}
	data, ok := fetchProbe(ctx, fetch, url)
	if !ok {
		return ""
	}
	if _, isImage := sniff(data); !isImage {
		return ""
	}
	return url
}
