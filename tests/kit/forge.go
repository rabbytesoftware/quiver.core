//go:build integration

package kit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type FakeForge interface {
	hosts.Host
	hosts.Forge
	Lookup(
		ns domain.Namespace,
	) (hosts.Host, bool)
	Calls() int64
}

type fakeForge struct {
	repos  map[domain.Namespace]ForgeRepo
	files  map[string][]byte
	assets map[string][]hosts.Asset
	server *httptest.Server
	calls  atomic.Int64
}

func NewFakeForge(
	t *testing.T,
	repos map[string]ForgeRepo,
) FakeForge {
	t.Helper()
	f := &fakeForge{
		repos:  map[domain.Namespace]ForgeRepo{},
		files:  map[string][]byte{},
		assets: map[string][]hosts.Asset{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	for ns, repo := range repos {
		f.register(t, domain.Namespace(ns), repo)
	}
	return f
}

func (f *fakeForge) Lookup(
	ns domain.Namespace,
) (hosts.Host, bool) {
	f.calls.Add(1)
	_, ok := f.repos[ns.BareNamespace()]
	if !ok {
		return nil, false
	}
	return f, true
}

func (f *fakeForge) Calls() int64 {
	return f.calls.Load()
}

func (f *fakeForge) RawFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	f.calls.Add(1)
	return f.server.URL + "/raw/" + string(ns.BareNamespace()) + "/" + ref + "/" + file, nil
}

func (f *fakeForge) DefaultBranches() []string {
	f.calls.Add(1)
	return []string{"main"}
}

func (f *fakeForge) LatestRelease(
	_ context.Context,
	ns domain.Namespace,
) (string, error) {
	f.calls.Add(1)
	repo, ok := f.repos[ns.BareNamespace()]
	if !ok || repo.latest() == "" {
		return "", fmt.Errorf("fake forge: latest release %s: %w", ns, hosts.ErrReleaseNotFound)
	}
	return repo.latest(), nil
}

func (f *fakeForge) ReleaseAssets(
	_ context.Context,
	ns domain.Namespace,
	tag string,
) ([]hosts.Asset, error) {
	f.calls.Add(1)
	assets, ok := f.assets[releaseKey(ns, tag)]
	if !ok {
		return nil, fmt.Errorf("fake forge: release %s %s: %w", ns, tag, hosts.ErrReleaseNotFound)
	}
	return slices.Clone(assets), nil
}

func (f *fakeForge) RepoPage(
	_ context.Context,
	ns domain.Namespace,
) (hosts.RepoPage, error) {
	f.calls.Add(1)
	repo, ok := f.repos[ns.BareNamespace()]
	if !ok {
		return hosts.RepoPage{}, fmt.Errorf("fake forge: repo page %s: %w", ns, hosts.ErrUnexpectedPage)
	}
	return hosts.RepoPage{Description: repo.Description}, nil
}

func (f *fakeForge) RawFile(
	_ context.Context,
	ns domain.Namespace,
	ref string,
	path string,
) ([]byte, error) {
	f.calls.Add(1)
	repo, ok := f.repos[ns.BareNamespace()]
	if !ok || path != "README.md" || !slices.Contains(repo.Tags, ref) {
		return nil, fmt.Errorf("fake forge: raw %s %s %s: %w", ns, ref, path, hosts.ErrRawNotFound)
	}
	return []byte(repo.Readme), nil
}

func (f *fakeForge) register(
	t *testing.T,
	ns domain.Namespace,
	repo ForgeRepo,
) {
	t.Helper()
	f.repos[ns] = repo
	for _, tag := range repo.Tags {
		f.assets[releaseKey(ns, tag)] = f.release(t, repo, tag)
	}
}

func (f *fakeForge) release(
	t *testing.T,
	repo ForgeRepo,
	tag string,
) []hosts.Asset {
	t.Helper()
	assets := make([]hosts.Asset, 0, len(domain.AllOS()))
	for _, platform := range domain.AllOS() {
		archive := tarball(t, repo.entryName(tag, platform), repo.script(tag))
		sum := sha256.Sum256(archive)
		name := repo.assetName(tag, platform)
		f.files["/download/"+tag+"/"+name] = archive
		assets = append(assets, hosts.Asset{
			Name:   name,
			URL:    f.server.URL + "/download/" + tag + "/" + name,
			Size:   int64(len(archive)),
			Digest: "sha256:" + hex.EncodeToString(sum[:]),
		})
	}
	return assets
}

func (f *fakeForge) serve(
	w http.ResponseWriter,
	r *http.Request,
) {
	body, ok := f.files[r.URL.Path]
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
