package fletcher_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	manifoldModels "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const (
	testNS   = domain.Namespace("github.com/acme/tool@v1.0.0")
	pagePath = "/acme/tool"
)

func noReleases() fletcher.Releases {
	return fletcher.Releases{
		LatestStable: func(_ context.Context, _ domain.Namespace) (string, error) {
			return "", nil
		},
		Channels: func(_ context.Context, _ domain.Namespace) ([]manifoldModels.ChannelInfo, error) {
			return nil, nil
		},
		DefaultBranch: func(_ context.Context, _ domain.Namespace) (string, string, error) {
			return "", "", errors.New("no default branch")
		},
	}
}

type pageHost struct {
	hosts.Host
	server *httptest.Server
}

func (h *pageHost) RawFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	return h.server.URL + "/raw/" + ref + "/" + file, nil
}

func (h *pageHost) BlobFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	return "https://github.com/acme/tool/blob/" + ref + "/" + file, nil
}

func (h *pageHost) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return ""
}

func (h *pageHost) RepoPageURL(
	_ domain.Namespace,
) string {
	return h.server.URL + pagePath
}

func (h *pageHost) DefaultBranches() []string {
	return []string{"main"}
}

func (h *pageHost) RepoMetadata(
	_ context.Context,
	_ domain.Namespace,
) (domain.RepoMetadata, error) {
	return domain.RepoMetadata{}, nil
}

func (h *pageHost) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return []domain.ReleaseAsset{{
		Name:   "tool-1.0.0-x86_64-unknown-linux-musl.tar.gz",
		URL:    "https://example.test/tool-1.0.0-x86_64-unknown-linux-musl.tar.gz",
		Digest: "sha256:00000000000000000000000000000000000000000000000000000000000000aa",
	}}, nil
}

func hostServing(
	t *testing.T,
	page http.HandlerFunc,
) hosts.Lookup {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pagePath {
			page(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	host := &pageHost{server: server}
	return func(_ domain.Namespace) (hosts.Host, bool) {
		return host, true
	}
}

func manifestMissing() error {
	return fmt.Errorf("%w: %s", resolver.ErrManifestNotFound, testNS)
}

func TestNew_UnknownHostIsAMissingManifest(t *testing.T) {
	fl := fletcher.New(nil, noReleases(), time.Second)

	raw, _, _, err := fl.Recover(context.Background(), domain.Namespace("example.org/acme/tool@v1.0.0"), manifestMissing())

	assert.Nil(t, raw)
	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.ErrorIs(t, err, fletcher.ErrNotFletchable)
	var nf fletcher.NotFletchableError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, fletcher.ReasonHostUnsupported, nf.Reason)
}

func TestNew_AnyOtherFailurePassesThrough(t *testing.T) {
	cause := fmt.Errorf("%w: HTTP 503", resolver.ErrFetchFailed)
	fl := fletcher.New(nil, noReleases(), time.Second)

	_, _, _, err := fl.Recover(context.Background(), testNS, cause)

	assert.Same(t, cause, err)
}

func TestNew_ExplicitTimeoutBoundsEveryFetch(t *testing.T) {
	lookup := hostServing(t, func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	fl := fletcher.New(lookup, noReleases(), 50*time.Millisecond)

	raw, _, _, err := fl.Recover(context.Background(), testNS, manifestMissing())

	assert.Nil(t, raw)
	assert.ErrorIs(t, err, resolver.ErrFetchFailed)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNew_ZeroTimeoutFallsBackToTheResolverDefault(t *testing.T) {
	lookup := hostServing(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><meta property="og:description" content="A tool."></head></html>`))
	})
	fl := fletcher.New(lookup, noReleases(), 0)

	raw, filename, _, err := fl.Recover(context.Background(), testNS, manifestMissing())

	require.NoError(t, err)
	assert.NotEmpty(t, raw)
	assert.Equal(t, "ARROW.md", filename)
}
