//go:build integration

package search_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

const (
	// fixtureHost is the host every fixture namespace lives under, so a
	// candidate built from it resolves against the in-memory git fixtures
	// instead of a real git host.
	fixtureHost = "quiver.test"
	// fixtureBranch is the branch go-git gives a freshly initialised fixture
	// repo, and therefore the only branch discovery can fetch a manifest from.
	fixtureBranch = "master"
	// secondHost stands in for a second git host in partial-failure tests.
	secondHost = "quiver.test2"
)

// fixtureNS builds the bare namespace of a fixture repo under testdata/arrows/.
func fixtureNS(fixture string) string {
	return fixtureHost + "/quiver-test/" + fixture
}

// discoveredNS is where discovery files a fixture: the bare namespace at the
// branch the provider named, which is the only revision a discovered arrow has.
func discoveredNS(fixture string) string {
	return fixtureNS(fixture) + "@" + fixtureBranch
}

// route is one canned provider answer, selected by query text.
type route struct {
	candidates []provider.Candidate
	err        error
	// gate holds Search until the test closes it. Holding the pass open is what
	// makes subscribing to its stream deterministic: nothing can be verified
	// before there is a subscriber to receive it.
	gate chan struct{}
}

// stubProvider answers from canned routes and records every query it was asked.
// It never opens a socket.
type stubProvider struct {
	host   string
	routes map[string]route

	mu    sync.Mutex
	calls []provider.SearchRequest
}

func newStubProvider(host string) *stubProvider {
	return &stubProvider{host: host, routes: make(map[string]route)}
}

// answer registers the candidates a query resolves to.
func (p *stubProvider) answer(query string, candidates ...provider.Candidate) *stubProvider {
	p.routes[query] = route{candidates: candidates}
	return p
}

// gated registers candidates that are withheld until the returned channel is
// closed.
func (p *stubProvider) gated(query string, candidates ...provider.Candidate) chan struct{} {
	gate := make(chan struct{})
	p.routes[query] = route{candidates: candidates, gate: gate}
	return gate
}

func (p *stubProvider) Host() string { return p.host }

func (p *stubProvider) CanSearch() bool { return true }

func (p *stubProvider) Search(
	ctx context.Context,
	req provider.SearchRequest,
) ([]provider.Candidate, error) {
	p.mu.Lock()
	p.calls = append(p.calls, req)
	p.mu.Unlock()

	r, ok := p.routes[req.Text]
	if !ok {
		return nil, nil
	}

	if r.gate != nil {
		select {
		case <-r.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if r.err != nil {
		return nil, r.err
	}
	return slices.Clone(r.candidates), nil
}

// The host questions below complete the provider contract. A discovery pass
// asks a provider to search and nothing else: the manifest that proves a
// candidate is fetched through the fixture-backed manifold.
func (p *stubProvider) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errNotAskedOfProvider
}

func (p *stubProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errNotAskedOfProvider
}

func (p *stubProvider) DefaultBranches() []string { return nil }

var errNotAskedOfProvider = errors.New("stub provider: discovery never asks this")

// searches reports how many times any query reached this host.
func (p *stubProvider) searches() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// topicsAsked returns the discovery markers the last request carried.
func (p *stubProvider) topicsAsked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return nil
	}
	return p.calls[len(p.calls)-1].Topics
}

// candidateFor builds the candidate a host would return for a fixture repo.
func candidateFor(fixture string, stars int) provider.Candidate {
	return provider.Candidate{
		Namespace:     domain.Namespace(fixtureNS(fixture)),
		Name:          fixture,
		Description:   "provider description for " + fixture,
		Stars:         stars,
		Source:        fixtureHost,
		DefaultBranch: fixtureBranch,
	}
}

// candidateForHost is candidateFor with an explicit answering host, used when a
// second provider answers for the same fixture namespaces.
func candidateForHost(host, fixture string, stars int) provider.Candidate {
	c := candidateFor(fixture, stars)
	c.Source = host
	return c
}

// missingCandidate names a repository that carries the marker but has no
// manifest behind it: the fetch 404s and the candidate is skipped.
func missingCandidate(name string) provider.Candidate {
	return provider.Candidate{
		Namespace:     domain.Namespace(fixtureHost + "/quiver-test/" + name),
		Name:          name,
		Source:        fixtureHost,
		DefaultBranch: fixtureBranch,
	}
}

// newHTTPProvider builds the real GitHub provider over a canned transport, so
// status classification, rate-limit headers and JSON decoding are the code
// under test while no socket is ever opened.
func newHTTPProvider(
	t *testing.T,
	host string,
	do provider.DoFunc,
) provider.Provider {
	t.Helper()
	p, err := provider.New(provider.Config{
		Host:      host,
		Kind:      metadata.KindGitHub,
		SearchURL: "https://" + host + "/api/search?q={query}",
		Timeout:   5 * time.Second,
		Do:        do,
	})
	require.NoError(t, err)
	return p
}

// respondJSON answers every request with one canned 200.
func respondJSON(body string) provider.DoFunc {
	return func(context.Context, fns.Request) (fns.Response, error) {
		return fns.Response{
			Status:  http.StatusOK,
			Headers: http.Header{},
			Body:    []byte(body),
		}, nil
	}
}

// respondStatus answers with a status and headers, which is how a rate limit,
// an unauthorized host and a broken host are told apart.
func respondStatus(status int, headers http.Header) provider.DoFunc {
	return func(context.Context, fns.Request) (fns.Response, error) {
		return fns.Response{Status: status, Headers: headers, Body: nil}, nil
	}
}

// respondErr answers with a transport failure, the shape a timeout or a refused
// connection reaches the provider as.
func respondErr(err error) provider.DoFunc {
	return func(context.Context, fns.Request) (fns.Response, error) {
		return fns.Response{}, err
	}
}

// countingManifold wraps the fixture-backed manifold and counts resolves, which
// are the only calls that would cost a network request in production. ParseArrow
// works on bytes already held, so it is counted apart.
type countingManifold struct {
	manifold.Manifold

	mu       sync.Mutex
	resolves int
	parses   int
}

func (m *countingManifold) ResolveArrow(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	m.mu.Lock()
	m.resolves++
	m.mu.Unlock()
	return m.Manifold.ResolveArrow(ctx, ns)
}

func (m *countingManifold) ParseArrow(
	data []byte,
) (*domain.Arrow, error) {
	m.mu.Lock()
	m.parses++
	m.mu.Unlock()
	return m.Manifold.ParseArrow(data)
}

func (m *countingManifold) counts() (resolves, parses int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolves, m.parses
}

// streamFrameTimeout bounds a read that is expected to deliver.
const streamFrameTimeout = 30 * time.Second

// quietWindow bounds a read that is expected to deliver nothing. It is only
// ever used once the pass it belongs to has completed, so no frame can still
// be in flight and the wait is a guard rather than a race.
const quietWindow = 300 * time.Millisecond

// readFrame reads one raw frame, failing the test if none arrives.
func readFrame(
	t *testing.T,
	conn *websocket.Conn,
) []byte {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(streamFrameTimeout)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "expected a frame on the discovery stream")
	return msg
}

// decodeStrictResult unmarshals a frame into the search result DTO with unknown
// fields rejected. This is the regression guard on the one-payload-type
// contract: a tagged union frame would carry a field the DTO does not have.
func decodeStrictResult(
	t *testing.T,
	raw []byte,
) searchResultFrame {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var frame searchResultFrame
	require.NoError(t, dec.Decode(&frame), "frame is not a search result: %s", raw)
	require.NotEmpty(t, frame.Namespace, "every frame must name an arrow: %s", raw)
	return frame
}

// searchResultFrame mirrors apidto.SearchResultDTO field for field. It is
// declared here rather than reused so that DisallowUnknownFields compares the
// wire shape against an independent description of it: a new field on the DTO
// has to be mirrored deliberately, and any other shape on the stream fails.
type searchResultFrame struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Media       struct {
		Icon   string `json:"icon"`
		Banner string `json:"banner"`
	} `json:"media"`
	Versions     []string `json:"versions"`
	CompatibleOS []string `json:"compatible_os"`
	Provenance   string   `json:"provenance"`
	Installed    bool     `json:"installed"`
	Known        bool     `json:"known"`
	Stars        int      `json:"stars"`
	Source       string   `json:"source"`
}

// readResults reads exactly want frames and returns them keyed by namespace.
func readResults(
	t *testing.T,
	conn *websocket.Conn,
	want int,
) map[string]searchResultFrame {
	t.Helper()
	got := make(map[string]searchResultFrame, want)
	for range want {
		frame := decodeStrictResult(t, readFrame(t, conn))
		got[frame.Namespace] = frame
	}
	return got
}

// requireQuiet asserts nothing more arrives. Call it only after the pass has
// completed, when no further frame can be produced — and only once per
// connection, last: gorilla makes a read error sticky, so the expired deadline
// closes the connection for reading.
func requireQuiet(
	t *testing.T,
	conn *websocket.Conn,
) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(quietWindow)))
	_, msg, err := conn.ReadMessage()
	require.Error(t, err, "unexpected extra frame: %s", msg)
}
