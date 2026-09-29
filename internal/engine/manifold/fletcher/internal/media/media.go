package media

import (
	"context"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const readmeIconLineLimit = 60

type Fetch func(
	ctx context.Context,
	url string,
) ([]byte, error)

type source struct {
	fetch Fetch
	urls  fileURLs
}

func newSource(
	fetch Fetch,
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) source {
	return source{
		fetch: fetch,
		urls:  fileURLsOf(host, ns, ref),
	}
}

func Resolve(
	ctx context.Context,
	fetch Fetch,
	host hosts.Host,
	ns domain.Namespace,
	ref string,
	socialImage string,
	readmeRaw []byte,
	probedIcon string,
) domain.ArrowMedia {
	src := newSource(fetch, host, ns, ref)
	return domain.ArrowMedia{
		Icon:   resolveIcon(ctx, src, readmeRaw, probedIcon),
		Banner: resolveBanner(ctx, src, socialImage, readmeRaw),
	}
}

func resolveIcon(
	ctx context.Context,
	src source,
	readmeRaw []byte,
	probedIcon string,
) string {
	if probedIcon != "" {
		return probedIcon
	}
	icon, _ := readmeIcon(ctx, src, readmeRaw)
	return icon
}

func resolveBanner(
	ctx context.Context,
	src source,
	socialImage string,
	readmeRaw []byte,
) string {
	if isBannerImage(ctx, src.fetch, socialImage) {
		return socialImage
	}
	banner, _ := readmeBanner(ctx, src, readmeRaw)
	return banner
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
	dim, ok := Sniff(data)
	return ok && isBannerShaped(dim)
}

func readmeIcon(
	ctx context.Context,
	src source,
	readmeRaw []byte,
) (string, bool) {
	for _, img := range readme.Images(readmeRaw) {
		if img.Line >= readmeIconLineLimit || readme.IsBadgeSrc(img.Src) {
			continue
		}
		if url, ok := tryImageCandidate(ctx, src, img.Src, isSquareDim); ok {
			return url, true
		}
	}
	return "", false
}

func readmeBanner(
	ctx context.Context,
	src source,
	readmeRaw []byte,
) (string, bool) {
	for _, img := range readme.Images(readmeRaw) {
		if readme.IsBadgeSrc(img.Src) {
			continue
		}
		if url, ok := tryImageCandidate(ctx, src, img.Src, isBannerShaped); ok {
			return url, true
		}
	}
	return "", false
}

func tryImageCandidate(
	ctx context.Context,
	src source,
	imageSrc string,
	accept func(Dimensions) bool,
) (string, bool) {
	path, ok := src.urls.resolve(imageSrc)
	if !ok {
		return "", false
	}
	url, ok := src.urls.pinned(path)
	if !ok {
		return "", false
	}
	data, ok := fetchProbe(ctx, src.fetch, url)
	if !ok {
		return "", false
	}
	dim, ok := Sniff(data)
	if !ok || !accept(dim) {
		return "", false
	}
	return url, true
}

func fetchProbe(
	ctx context.Context,
	fetch Fetch,
	url string,
) ([]byte, bool) {
	data, err := fetch(ctx, url)
	return data, err == nil
}

func isSquareDim(
	dim Dimensions,
) bool {
	return dim.Width == dim.Height
}

func isBannerShaped(
	dim Dimensions,
) bool {
	if dim.Width >= 2*dim.Height {
		return true
	}
	return dim.Width > dim.Height && dim.Width >= 400 && dim.Height >= 200
}

func isRelativeSrc(
	src string,
) bool {
	if strings.HasPrefix(src, "//") {
		return false
	}
	return !strings.Contains(src, "://")
}

func cleanPath(
	path string,
) string {
	path = strings.TrimPrefix(path, "./")
	return strings.TrimPrefix(path, "/")
}
