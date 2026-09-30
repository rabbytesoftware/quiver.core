package gather

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fnsconfig "github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

func endlessServer(
	t *testing.T,
	sent int,
) (*httptest.Server, chan struct{}) {
	t.Helper()
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), sent))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(released)
	}))
	t.Cleanup(server.Close)
	return server, released
}

func TestFetcher_Prefix_ReadsAtMostTheLimitOfAnEndlessBody(t *testing.T) {
	server, released := endlessServer(t, maxProbeBytes+1024)
	f := fetcher{timeout: 10 * time.Second}

	data, err := f.prefixOf(maxProbeBytes)(context.Background(), server.URL+"/huge.png")

	require.NoError(t, err)
	assert.Len(t, data, maxProbeBytes)
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection was kept open after the limit")
	}
}

func TestFetcher_Document(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("# readme"))
		case "/broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f := fetcher{timeout: 10 * time.Second}

	data, err := f.document(context.Background(), server.URL+"/ok")
	require.NoError(t, err)
	assert.Equal(t, "# readme", string(data))

	testCases := []struct {
		name    string
		path    string
		want    error
		notWant error
	}{
		{name: "missing", path: "/missing", want: resolver.ErrNotFound, notWant: resolver.ErrFetchFailed},
		{name: "server error", path: "/broken", want: resolver.ErrFetchFailed, notWant: resolver.ErrNotFound},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.document(context.Background(), server.URL+tc.path)

			require.ErrorIs(t, err, tc.want)
			assert.NotErrorIs(t, err, tc.notWant)
		})
	}
}

func TestFetcher_Prefix_NonSuccessFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	_, err := fetcher{timeout: time.Second}.prefix(context.Background(), server.URL+"/x", maxProbeBytes)

	require.ErrorIs(t, err, resolver.ErrFetchFailed)
}

func TestFetcher_Prefix_BodyReadFailureFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	t.Cleanup(server.Close)

	_, err := fetcher{timeout: time.Second}.prefix(context.Background(), server.URL+"/x", maxProbeBytes)

	require.ErrorIs(t, err, resolver.ErrFetchFailed)
}

func TestFetcher_RefusesAnythingButHTTP(t *testing.T) {
	f := fetcher{timeout: time.Second}
	for _, rawURL := range []string{
		"file:///etc/passwd",
		"/etc/passwd",
		"README.md",
		"HTTP://example.test/x",
		"http://",
		"https://%zz",
		"",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, docErr := f.document(context.Background(), rawURL)
			_, prefixErr := f.prefix(context.Background(), rawURL, maxProbeBytes)

			require.ErrorIs(t, docErr, resolver.ErrFetchFailed)
			require.ErrorIs(t, prefixErr, resolver.ErrFetchFailed)
		})
	}
}

func TestFetcher_ClientTimeoutIsDisabled(t *testing.T) {
	cfg := fnsconfig.Default()
	for _, opt := range contextBound() {
		opt(&cfg)
	}

	assert.Zero(t, cfg.HTTPClient.Timeout)
}

func TestFetcher_TheFetchTimeoutGovernsThroughTheContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	f := fetcher{timeout: 50 * time.Millisecond}

	reads := map[string]func() error{
		"document": func() error {
			_, err := f.document(context.Background(), server.URL+"/slow")
			return err
		},
		"prefix": func() error {
			_, err := f.prefix(context.Background(), server.URL+"/slow", maxProbeBytes)
			return err
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- read() }()

			select {
			case err := <-done:
				require.ErrorIs(t, err, resolver.ErrFetchFailed)
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(10 * time.Second):
				t.Fatal("the fetch timeout did not end a stalled request")
			}
		})
	}
}
