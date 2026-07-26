package fns_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	fnsconfig "github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns/mocks"
)

func TestDo_RemoteURLDispatchesToRemote(t *testing.T) {
	client := mocks.NewMockHTTPClient(mocks.RoundTripFunc(
		func(req *http.Request) (*http.Response, error) {
			h := http.Header{}
			h.Set("X-Test", "yes")
			return &http.Response{
				StatusCode: http.StatusTeapot,
				Header:     h,
				Body:       io.NopCloser(strings.NewReader("body")),
			}, nil
		},
	))

	resp, err := fns.Do(
		context.Background(),
		fns.Request{URL: "https://example.com/x"},
		fnsconfig.WithHTTPClient(client),
	)

	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, resp.Status)
	assert.Equal(t, "yes", resp.Headers.Get("X-Test"))
	assert.Equal(t, "body", string(resp.Body))
}

func TestDo_LocalPathIsUnsupported(t *testing.T) {
	_, err := fns.Do(context.Background(), fns.Request{URL: "/tmp/whatever"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}
