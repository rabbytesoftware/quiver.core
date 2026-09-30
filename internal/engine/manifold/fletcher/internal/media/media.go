package media

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Fetch func(
	ctx context.Context,
	url string,
) ([]byte, error)

// Resolve settles an arrow's media: the repository's own icon when one was
// found, else the owner's avatar, and the social preview as banner when it is
// banner-shaped. Media never fails a draft, so a miss is an empty field.
func Resolve(
	ctx context.Context,
	fetch Fetch,
	socialImage string,
	repoIcon string,
	avatar string,
) domain.ArrowMedia {
	icon := repoIcon
	if icon == "" {
		icon = avatar
	}
	banner := ""
	if isBannerImage(ctx, fetch, socialImage) {
		banner = socialImage
	}
	return domain.ArrowMedia{Icon: icon, Banner: banner}
}

func isBannerImage(
	ctx context.Context,
	fetch Fetch,
	url string,
) bool {
	if url == "" {
		return false
	}
	data, ok := fetchProbe(ctx, fetch, url)
	if !ok {
		return false
	}
	dim, ok := sniff(data)
	return ok && isBannerShaped(dim)
}

func fetchProbe(
	ctx context.Context,
	fetch Fetch,
	url string,
) ([]byte, bool) {
	data, err := fetch(ctx, url)
	return data, err == nil
}

func isBannerShaped(
	dim dimensions,
) bool {
	if dim.Width >= 2*dim.Height {
		return true
	}
	return dim.Width > dim.Height && dim.Width >= 400 && dim.Height >= 200
}
