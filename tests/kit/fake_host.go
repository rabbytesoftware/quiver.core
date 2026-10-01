//go:build integration

package kit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type FakeHost interface {
	hosts.Host
	Lookup(
		ns domain.Namespace,
	) (hosts.Host, bool)
	Calls() int64
	MetadataCalls() int64
	// Downloads counts the asset bodies the host has served; a HEAD probe is not one.
	Downloads() int64
	// Republish replaces a release's archives with different bytes and reports
	// their new digests, the way a rolling tag is re-published.
	Republish(
		t *testing.T,
		ns domain.Namespace,
		tag string,
	)
	// Corrupt changes the bytes a release serves without changing the digests
	// it reports, so no manifest can ever match what gets downloaded.
	Corrupt(
		ns domain.Namespace,
		tag string,
	)
}

var errNoFakeMetadata = errors.New("fake host: no metadata")

type fakeHost struct {
	repos     map[domain.Namespace]HostRepo
	mu        sync.RWMutex
	files     map[string][]byte
	assets    map[string][]domain.ReleaseAsset
	revisions map[string]int
	server    *httptest.Server
	calls     atomic.Int64
	meta      atomic.Int64
	downloads atomic.Int64
}

func NewFakeHost(
	t *testing.T,
	repos map[string]HostRepo,
) FakeHost {
	t.Helper()
	f := &fakeHost{
		repos:  map[domain.Namespace]HostRepo{},
		files:  map[string][]byte{},
		assets: map[string][]domain.ReleaseAsset{},

		revisions: map[string]int{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	for ns, repo := range repos {
		f.register(t, domain.Namespace(ns), repo)
	}
	return f
}

func (f *fakeHost) Lookup(
	ns domain.Namespace,
) (hosts.Host, bool) {
	_, ok := f.repos[ns.BareNamespace()]
	if !ok {
		return nil, false
	}
	return f, true
}

func (f *fakeHost) Calls() int64 {
	return f.calls.Load()
}

func (f *fakeHost) MetadataCalls() int64 {
	return f.meta.Load()
}

func (f *fakeHost) Downloads() int64 {
	return f.downloads.Load()
}

func (f *fakeHost) Republish(
	t *testing.T,
	ns domain.Namespace,
	tag string,
) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	key := releaseKey(ns, tag)
	f.revisions[key]++
	f.assets[key] = f.release(t, f.repos[ns.BareNamespace()], tag, f.revisions[key])
}

func (f *fakeHost) Corrupt(
	ns domain.Namespace,
	tag string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, asset := range f.assets[releaseKey(ns, tag)] {
		f.files["/download/"+tag+"/"+asset.Name] = []byte("corrupted in transit")
	}
}

func (f *fakeHost) RepoMetadata(
	_ context.Context,
	ns domain.Namespace,
) (domain.RepoMetadata, error) {
	f.meta.Add(1)
	repo, ok := f.repos[ns.BareNamespace()]
	if !ok || repo.APIDescription == "" && repo.AvatarURL == "" {
		return domain.RepoMetadata{}, fmt.Errorf("fake host: repo metadata %s: %w", ns, errNoFakeMetadata)
	}
	return domain.RepoMetadata{Description: repo.APIDescription, AvatarURL: repo.AvatarURL}, nil
}

func (f *fakeHost) RawFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	f.calls.Add(1)
	return f.server.URL + "/raw/" + string(ns.BareNamespace()) + "/" + ref + "/" + file, nil
}

func (f *fakeHost) BlobFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	f.calls.Add(1)
	segments := strings.Split(string(ns.BareNamespace()), domain.NamespaceSeparator)
	if len(segments) < 3 {
		return "", fmt.Errorf("fake host: blob %s: invalid namespace", ns)
	}
	return strings.NewReplacer(
		"{user}", segments[1],
		"{repo}", segments[2],
		"{branch}", ref,
		"{file}", file,
	).Replace(metadata.GetPlatforms()["github.com"].BlobURL), nil
}

func (f *fakeHost) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return ""
}

func (f *fakeHost) RepoPageURL(
	ns domain.Namespace,
) string {
	f.calls.Add(1)
	return f.server.URL + "/page/" + string(ns.BareNamespace())
}

func (f *fakeHost) DefaultBranches() []string {
	return []string{"main"}
}

func (f *fakeHost) ReleaseAssets(
	_ context.Context,
	ns domain.Namespace,
	tag string,
) ([]domain.ReleaseAsset, error) {
	f.calls.Add(1)
	f.mu.RLock()
	defer f.mu.RUnlock()
	return slices.Clone(f.assets[releaseKey(ns, tag)]), nil
}

func (f *fakeHost) register(
	t *testing.T,
	ns domain.Namespace,
	repo HostRepo,
) {
	t.Helper()
	f.repos[ns] = repo
	f.files["/page/"+string(ns)] = []byte(fmt.Sprintf(
		`<html><head><meta property="og:description" content=%q></head></html>`,
		repo.Description,
	))
	for _, ref := range append([]string{"main"}, repo.Tags...) {
		for path, body := range repo.Files {
			f.files["/raw/"+string(ns)+"/"+ref+"/"+path] = body
		}
	}
	for _, tag := range repo.Tags {
		f.assets[releaseKey(ns, tag)] = f.release(t, repo, tag, 0)
		f.files["/raw/"+string(ns)+"/"+tag+"/README.md"] = []byte(repo.Readme)
	}
}

func (f *fakeHost) release(
	t *testing.T,
	repo HostRepo,
	tag string,
	revision int,
) []domain.ReleaseAsset {
	t.Helper()
	assets := make([]domain.ReleaseAsset, 0, len(domain.AllOS()))
	for _, platform := range domain.AllOS() {
		archive := tarball(t, repo.Binary, repo.script(tag, revision))
		sum := sha256.Sum256(archive)
		name := repo.assetName(tag, platform)
		f.files["/download/"+tag+"/"+name] = archive
		assets = append(assets, domain.ReleaseAsset{
			Name:   name,
			URL:    f.server.URL + "/download/" + tag + "/" + name,
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
		})
	}
	return assets
}

func (f *fakeHost) serve(
	w http.ResponseWriter,
	r *http.Request,
) {
	if strings.HasPrefix(r.URL.Path, "/download/") {
		if r.Method == http.MethodGet {
			f.downloads.Add(1)
		}
	} else {
		f.calls.Add(1)
	}
	f.mu.RLock()
	body, ok := f.files[r.URL.Path]
	f.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func releaseKey(
	ns domain.Namespace,
	tag string,
) string {
	return string(ns.BareNamespace()) + "@" + tag
}

func tarball(
	t *testing.T,
	name string,
	content []byte,
) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("tarball %s: header: %v", name, err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tarball %s: write: %v", name, err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tarball %s: close tar: %v", name, err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("tarball %s: close gzip: %v", name, err)
	}
	return buf.Bytes()
}
