package providers

import (
	"errors"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type gitlabAsset struct {
	domain.ReleaseAsset
	linkURL string
}

func (a gitlabAsset) names() []string {
	names := make([]string, 0, 3)
	for _, name := range []string{strings.ToLower(a.Name), urlBase(a.URL), urlBase(a.linkURL)} {
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func (a gitlabAsset) secure() bool {
	return isHTTPS(a.linkURL) && isHTTPS(a.URL)
}

func (a gitlabAsset) sumTarget() (string, bool) {
	for _, name := range a.names() {
		if target, ok := checksumTarget(name); ok {
			return target, true
		}
	}
	return "", false
}

func (a gitlabAsset) needsDigest() bool {
	if a.Digest != "" || !a.secure() {
		return false
	}
	_, isSumFile := a.sumTarget()
	return !isSumFile
}

func publicAssets(
	assets []gitlabAsset,
) []domain.ReleaseAsset {
	public := make([]domain.ReleaseAsset, len(assets))
	for i, asset := range assets {
		public[i] = asset.ReleaseAsset
		if !asset.secure() {
			public[i].Digest = ""
		}
	}
	return public
}

func urlBase(
	rawURL string,
) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(parsed.Path)
	if base == "." || base == "/" {
		return ""
	}
	return strings.ToLower(base)
}

func isHTTPS(
	rawURL string,
) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

type gitlabLink struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url"`
}

func (l gitlabLink) asset() gitlabAsset {
	assetURL := l.DirectAssetURL
	if assetURL == "" {
		assetURL = l.URL
	}
	return gitlabAsset{
		ReleaseAsset: domain.ReleaseAsset{
			Name: l.Name,
			URL:  assetURL,
		},
		linkURL: l.URL,
	}
}

func absoluteURL(
	base string,
	href string,
) string {
	if href == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return href
	}
	return resolveHref(parsed, href)
}

type gitlabPackage struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type gitlabPackageFile struct {
	FileName   string `json:"file_name"`
	FileSHA256 string `json:"file_sha256"`
}

type gitlabRelease struct {
	Assets struct {
		Links []gitlabLink `json:"links"`
	} `json:"assets"`
}

type packageRef struct {
	project string
	name    string
	version string
}

func genericPackageFile(
	project string,
	filePath string,
) (packageRef, string, bool) {
	segments := strings.Split(filePath, "/")
	if project == "" || len(segments) != 3 {
		return packageRef{}, "", false
	}

	name, errName := url.PathUnescape(segments[0])
	version, errVersion := url.PathUnescape(segments[1])
	file, errFile := url.PathUnescape(segments[2])
	if errors.Join(errName, errVersion, errFile) != nil {
		return packageRef{}, "", false
	}

	ref := packageRef{project: project, name: name, version: version}
	return ref, file, name != "" && version != "" && file != ""
}

func (r packageRef) match(
	packages []gitlabPackage,
) (int64, bool) {
	for _, pkg := range packages {
		if pkg.Name == r.name && pkg.Version == r.version {
			return pkg.ID, true
		}
	}
	return 0, false
}
