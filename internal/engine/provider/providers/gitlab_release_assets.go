package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	gitlabProjectPlaceholder = "{project}"
	genericPackageSegment    = "/generic/"
	packageFilesPerPage      = 100
	maxPackageFilePages      = 10
	maxChecksumRedirects     = 5
)

func (p *gitlabProvider) ReleaseAssets(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) ([]domain.ReleaseAsset, error) {
	releaseURL, err := tagURL(p.releaseAPIURL, ns, tag)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}

	var release gitlabRelease
	found, err := p.getJSON(ctx, releaseURL, &release)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}
	if !found {
		return []domain.ReleaseAsset{}, nil
	}

	assets, err := p.digestedAssets(ctx, release.Assets.Links)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}
	return assets, nil
}

func (p *gitlabProvider) getJSON(
	ctx context.Context,
	rawURL string,
	into any,
) (bool, error) {
	resp, err := p.transport.fetch(ctx, rawURL, p.headers())
	if err != nil {
		return false, err
	}
	if resp.Status == http.StatusNotFound {
		return false, nil
	}
	if err := p.transport.classify(resp); err != nil {
		return false, err
	}
	if err := json.Unmarshal(resp.Body, into); err != nil {
		return false, fmt.Errorf("decode %s: %w", rawURL, ErrUnexpectedPage)
	}
	return true, nil
}

func (p *gitlabProvider) digestedAssets(
	ctx context.Context,
	links []gitlabLink,
) ([]domain.ReleaseAsset, error) {
	assets := make([]gitlabAsset, len(links))
	for i, link := range links {
		assets[i] = link.asset()
	}

	if err := p.fillPackageDigests(ctx, assets); err != nil {
		return nil, fmt.Errorf("package digests: %w", err)
	}
	p.fillChecksumDigests(ctx, assets)
	return publicAssets(assets), nil
}

func (p *gitlabProvider) fillPackageDigests(
	ctx context.Context,
	assets []gitlabAsset,
) error {
	cache := make(map[packageRef]map[string]string)
	for i := range assets {
		ref, file, ok := p.packageFileOf(assets[i].linkURL)
		if !ok {
			continue
		}
		digests, err := p.cachedPackageDigests(ctx, cache, ref)
		if err != nil {
			return err
		}
		assets[i].Digest = digests[file]
	}
	return nil
}

func (p *gitlabProvider) packageFileOf(
	linkURL string,
) (packageRef, string, bool) {
	prefix, suffix, ok := strings.Cut(p.packagesAPIURL, gitlabProjectPlaceholder)
	if !ok {
		return packageRef{}, "", false
	}
	rest, ok := strings.CutPrefix(linkURL, prefix)
	if !ok {
		return packageRef{}, "", false
	}
	project, filePath, ok := strings.Cut(rest, suffix+genericPackageSegment)
	if !ok {
		return packageRef{}, "", false
	}
	return genericPackageFile(project, filePath)
}

func (p *gitlabProvider) cachedPackageDigests(
	ctx context.Context,
	cache map[packageRef]map[string]string,
	ref packageRef,
) (map[string]string, error) {
	if digests, ok := cache[ref]; ok {
		return digests, nil
	}

	digests, err := p.packageDigests(ctx, ref)
	if err != nil {
		return nil, err
	}
	cache[ref] = digests
	return digests, nil
}

func (p *gitlabProvider) packageDigests(
	ctx context.Context,
	ref packageRef,
) (map[string]string, error) {
	var packages []gitlabPackage
	found, err := p.getJSON(ctx, p.packagesQueryURL(ref), &packages)
	if isDenied(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	id, ok := ref.match(packages)
	if !found || !ok {
		return map[string]string{}, nil
	}
	return p.packageFileDigests(ctx, ref, id)
}

func (p *gitlabProvider) packageFileDigests(
	ctx context.Context,
	ref packageRef,
	id int64,
) (map[string]string, error) {
	digests := make(map[string]string)
	for page := 1; page <= maxPackageFilePages; page++ {
		var files []gitlabPackageFile
		found, err := p.getJSON(ctx, p.packageFilesURL(ref, id, page), &files)
		if isDenied(err) {
			return digests, nil
		}
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if digest := sha256Digest(file.FileSHA256); digest != "" {
				digests[file.FileName] = digest
			}
		}
		if !found || len(files) < packageFilesPerPage {
			return digests, nil
		}
	}
	return digests, nil
}

func (p *gitlabProvider) packagesBaseURL(
	ref packageRef,
) string {
	return strings.ReplaceAll(p.packagesAPIURL, gitlabProjectPlaceholder, ref.project)
}

func (p *gitlabProvider) packagesQueryURL(
	ref packageRef,
) string {
	query := url.Values{}
	query.Set("package_type", "generic")
	query.Set("package_name", ref.name)
	query.Set("package_version", ref.version)
	return p.packagesBaseURL(ref) + "?" + query.Encode()
}

func (p *gitlabProvider) packageFilesURL(
	ref packageRef,
	id int64,
	page int,
) string {
	query := url.Values{}
	query.Set("per_page", strconv.Itoa(packageFilesPerPage))
	query.Set("page", strconv.Itoa(page))
	return p.packagesBaseURL(ref) + "/" + strconv.FormatInt(id, 10) + "/package_files?" + query.Encode()
}

func isDenied(
	err error,
) bool {
	var denied *UnauthorizedError
	return errors.As(err, &denied)
}

func (p *gitlabProvider) fillChecksumDigests(
	ctx context.Context,
	assets []gitlabAsset,
) {
	pending := pendingNames(assets)
	if len(pending) == 0 {
		return
	}

	sums := newChecksumSet()
	for _, asset := range assets {
		target, ok := asset.sumTarget()
		if !ok || !asset.secure() || !wantedSumFile(target, pending) {
			continue
		}
		parseChecksums(sums, p.checksumBody(ctx, asset.URL), target)
	}

	for i := range assets {
		if assets[i].needsDigest() {
			assets[i].Digest = sums.lookup(assets[i].names())
		}
	}
}

func (p *gitlabProvider) checksumBody(
	ctx context.Context,
	rawURL string,
) []byte {
	for hop := 0; hop <= maxChecksumRedirects; hop++ {
		if !isHTTPS(rawURL) {
			return nil
		}
		resp, err := p.transport.redirect(ctx, rawURL)
		if err != nil {
			return nil
		}
		location := resp.Headers.Get("Location")
		if !isRedirect(resp.Status) || location == "" {
			return checksumPayload(resp)
		}
		rawURL = absoluteURL(rawURL, location)
	}
	return nil
}

func pendingNames(
	assets []gitlabAsset,
) map[string]struct{} {
	pending := make(map[string]struct{})
	for _, asset := range assets {
		if !asset.needsDigest() {
			continue
		}
		for _, name := range asset.names() {
			pending[name] = struct{}{}
		}
	}
	return pending
}

func wantedSumFile(
	target string,
	pending map[string]struct{},
) bool {
	if target == "" {
		return true
	}
	_, ok := pending[target]
	return ok
}

func checksumPayload(
	resp fns.Response,
) []byte {
	if resp.Status != http.StatusOK || len(resp.Body) > maxChecksumBytes {
		return nil
	}
	return resp.Body
}

func isRedirect(
	status int,
) bool {
	return status >= http.StatusMultipleChoices && status < http.StatusBadRequest
}
