package media

import (
	"context"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const maxProbeBytes = 64 * 1024

const readmeIconLineLimit = 60

func Resolve(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	page hosts.RepoPage,
	readmeRaw []byte,
) domain.ArrowMedia {
	owner, repo := ownerRepo(ns)
	return domain.ArrowMedia{
		Icon:   resolveIcon(ctx, forge, ns, ref, owner, repo, page, readmeRaw),
		Banner: resolveBanner(ctx, forge, ns, ref, owner, repo, page, readmeRaw),
	}
}

func ownerRepo(
	ns domain.Namespace,
) (string, string) {
	parts := strings.Split(string(ns.BareNamespace()), "/")
	if len(parts) < 3 {
		return "", ""
	}
	return parts[1], parts[2]
}

func resolveIcon(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
	page hosts.RepoPage,
	readmeRaw []byte,
) string {
	if icon, ok := probedIcon(ctx, forge, ns, ref, owner, repo); ok {
		return icon
	}
	if icon, ok := readmeIcon(ctx, forge, ns, ref, owner, repo, readmeRaw); ok {
		return icon
	}
	if page.OwnerIsOrg && page.OwnerAvatar != "" {
		return page.OwnerAvatar
	}
	return ""
}

func resolveBanner(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
	page hosts.RepoPage,
	readmeRaw []byte,
) string {
	if page.CustomSocialImage && page.SocialImage != "" {
		return page.SocialImage
	}
	if banner, ok := readmeBanner(ctx, forge, ns, ref, owner, repo, readmeRaw); ok {
		return banner
	}
	return ""
}

func probedIcon(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
) (string, bool) {
	for _, path := range IconProbePaths() {
		data, ok := fetchCapped(ctx, forge, ns, ref, path)
		if !ok {
			continue
		}
		if !acceptProbedIcon(path, data) {
			continue
		}
		return pinnedRawURL(owner, repo, ref, path), true
	}
	return "", false
}

func readmeIcon(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
	readmeRaw []byte,
) (string, bool) {
	for _, img := range readme.Images(readmeRaw) {
		if img.Line >= readmeIconLineLimit || readme.IsBadgeSrc(img.Src) {
			continue
		}
		if url, ok := tryImageCandidate(ctx, forge, ns, ref, owner, repo, img.Src, isSquareDim); ok {
			return url, true
		}
	}
	return "", false
}

func readmeBanner(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
	readmeRaw []byte,
) (string, bool) {
	for _, img := range readme.Images(readmeRaw) {
		if readme.IsBadgeSrc(img.Src) {
			continue
		}
		if url, ok := tryImageCandidate(ctx, forge, ns, ref, owner, repo, img.Src, isBannerShaped); ok {
			return url, true
		}
	}
	return "", false
}

func tryImageCandidate(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	owner string,
	repo string,
	src string,
	accept func(Dimensions) bool,
) (string, bool) {
	path, ok := resolvePath(src, owner, repo)
	if !ok {
		return "", false
	}
	data, ok := fetchCapped(ctx, forge, ns, ref, path)
	if !ok {
		return "", false
	}
	dim, ok := Sniff(data)
	if !ok {
		return "", false
	}
	if !accept(dim) {
		return "", false
	}
	return pinnedRawURL(owner, repo, ref, path), true
}

func fetchCapped(
	ctx context.Context,
	forge hosts.Forge,
	ns domain.Namespace,
	ref string,
	path string,
) ([]byte, bool) {
	data, err := forge.RawFile(ctx, ns, ref, path)
	if err != nil {
		return nil, false
	}
	if len(data) > maxProbeBytes {
		data = data[:maxProbeBytes]
	}
	return data, true
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

func afterRef(
	remainder string,
) (string, bool) {
	idx := strings.IndexByte(remainder, '/')
	if idx < 0 {
		return "", false
	}
	return remainder[idx+1:], true
}

func resolvePath(
	src string,
	owner string,
	repo string,
) (string, bool) {
	if isRelativeSrc(src) {
		return cleanPath(src), true
	}
	rawPrefix := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/", owner, repo)
	if strings.HasPrefix(src, rawPrefix) {
		return afterRef(strings.TrimPrefix(src, rawPrefix))
	}
	blobPrefix := fmt.Sprintf("https://github.com/%s/%s/blob/", owner, repo)
	if strings.HasPrefix(src, blobPrefix) {
		return afterRef(strings.TrimPrefix(src, blobPrefix))
	}
	return "", false
}

func pinnedRawURL(
	owner string,
	repo string,
	ref string,
	path string,
) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, path)
}
