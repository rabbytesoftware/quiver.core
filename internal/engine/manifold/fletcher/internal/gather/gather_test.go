package gather_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const deadlockGuard = 10 * time.Second

type rendezvous struct {
	pageWaitsFor []chan struct{}
	signals      map[string]chan struct{}
	pageSaw      chan error
	pageArrived  chan struct{}
	once         sync.Map
}

func (r *rendezvous) onRequest(
	req *http.Request,
) bool {
	if signal, ok := r.signals[req.URL.Path]; ok {
		once, _ := r.once.LoadOrStore(req.URL.Path, &sync.Once{})
		once.(*sync.Once).Do(func() { close(signal) })
	}
	if req.URL.Path != repoPagePath {
		return true
	}
	if r.pageArrived != nil {
		close(r.pageArrived)
	}
	for _, wait := range r.pageWaitsFor {
		select {
		case <-wait:
		case <-req.Context().Done():
			r.pageSaw <- req.Context().Err()
			return false
		}
	}
	r.pageSaw <- nil
	return true
}

func TestDrafter_Draft_FetchesPageReadmeAndIconsConcurrently(t *testing.T) {
	iconAsked := make(chan struct{})
	readmeAsked := make(chan struct{})
	meet := &rendezvous{
		pageWaitsFor: []chan struct{}{iconAsked, readmeAsked},
		signals: map[string]chan struct{}{
			"/raw/" + testTag + "/src-tauri/icons/icon.png": iconAsked,
			"/raw/" + testTag + "/README.md":                readmeAsked,
		},
		pageSaw: make(chan error, 1),
	}
	host := &stubHost{
		assets:    realAssets(),
		page:      pageWith("og:description", "Tool description."),
		files:     map[string][]byte{"README.md": []byte("# Tool\n\nFast.\n")},
		onRequest: meet.onRequest,
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadlockGuard)
	defer cancel()

	manifest, err := newDrafter(t, host, picker.New()).Draft(ctx, testNS, testTag)

	require.NoError(t, err)
	require.NoError(t, <-meet.pageSaw)
	assert.Contains(t, parse(t, manifest).Readme, "Fast.")
}

func TestDrafter_Draft_AssetFailureCancelsTheOtherFetches(t *testing.T) {
	meet := &rendezvous{
		pageWaitsFor: []chan struct{}{make(chan struct{})},
		pageSaw:      make(chan error, 1),
		pageArrived:  make(chan struct{}),
	}
	host := &stubHost{
		assetsErr:  errBoom,
		assetsWait: meet.pageArrived,
		page:       pageWith("og:description", "d"),
		onRequest:  meet.onRequest,
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadlockGuard)
	defer cancel()

	_, err := newDrafter(t, host, picker.New()).Draft(ctx, testNS, testTag)

	require.ErrorIs(t, err, errBoom)
	assert.NotErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, <-meet.pageSaw, context.Canceled)
}

func TestDrafter_Draft_AssetFailureWinsOverPageAndReadme(t *testing.T) {
	host := &stubHost{
		assets:     []domain.ReleaseAsset{},
		pageStatus: http.StatusInternalServerError,
		fileStatus: map[string]int{"README.md": http.StatusInternalServerError},
	}

	_, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)

	require.ErrorIs(t, err, models.ErrNotFletchable)
	assert.NotErrorIs(t, err, resolver.ErrFetchFailed)
}
