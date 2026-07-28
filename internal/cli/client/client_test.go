package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
)

// ─── test server ─────────────────────────────────────────────────────────────

// record captures the last request the fake daemon saw.
type record struct {
	method string
	path   string
	query  string
	body   string
}

func fakeDaemon(t *testing.T, status int, payload string) (*httptest.Server, *record) {
	t.Helper()
	rec := &record{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.EscapedPath()
		rec.query = r.URL.RawQuery
		var buf [4096]byte
		n, _ := r.Body.Read(buf[:])
		rec.body = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func newClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.New(srv.URL)
	require.NoError(t, err)
	return c
}

// ─── New ─────────────────────────────────────────────────────────────────────

func TestNew_RejectsEmptyServer(t *testing.T) {
	_, err := client.New("")
	assert.Error(t, err)
}

func TestNew_RejectsUnknownScheme(t *testing.T) {
	_, err := client.New("ftp://example.com")
	assert.Error(t, err)
}

func TestNew_AcceptsTCPScheme(t *testing.T) {
	c, err := client.New("tcp://127.0.0.1:40257")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNew_AcceptsUnixScheme(t *testing.T) {
	c, err := client.New("unix:///tmp/quiver.sock")
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// ─── unix socket transport ───────────────────────────────────────────────────

func TestClient_UnixSocketRoundtrip(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "quiver.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	c, err := client.New("unix://" + sock)
	require.NoError(t, err)

	err = c.Health(context.Background())
	assert.NoError(t, err)
}

// ─── system ──────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"status":"ok"}`)
	c := newClient(t, srv)

	err := c.Health(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/v0/health", rec.path)
	assert.Equal(t, http.MethodGet, rec.method)
}

func TestHealth_DaemonDown(t *testing.T) {
	c, err := client.New("tcp://127.0.0.1:1") // nothing listens on port 1
	require.NoError(t, err)

	err = c.Health(context.Background())
	assert.Error(t, err)
	assert.Equal(t, 3, client.ExitCode(err))
}

func TestVersions_ReturnsBuildInfo(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":{"version":"26.5","build_id":"83","api":{"supported":["v0"],"latest":"v0"}}}`)
	c := newClient(t, srv)

	v, err := c.Versions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/versions", rec.path)
	assert.Equal(t, "26.5", v.Version)
	assert.Equal(t, "83", v.BuildID)
	assert.Equal(t, []string{"v0"}, v.API.Supported)
}

// ─── arrows ──────────────────────────────────────────────────────────────────

func TestListArrows_DecodesItems(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":[{"namespace":"github.com/user/a","name":"A"},{"namespace":"github.com/user/b","name":"B"}]}`)
	c := newClient(t, srv)

	arrows, err := c.ListArrows(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "/v0/arrow", rec.path)
	require.Len(t, arrows, 2)
	assert.Equal(t, "github.com/user/a", arrows[0].Namespace)
}

func TestListArrows_UserInstalledQuery(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true,"data":[]}`)
	c := newClient(t, srv)

	all := false
	_, err := c.ListArrows(context.Background(), &all)
	require.NoError(t, err)
	assert.Equal(t, "user_installed=false", rec.query)
}

func TestGetArrow_EncodesNamespace(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":{"namespace":"github.com/user/a@v1","state":"ready"}}`)
	c := newClient(t, srv)

	detail, err := c.GetArrow(context.Background(), "github.com/user/a@v1")
	require.NoError(t, err)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa@v1", rec.path)
	assert.Equal(t, "ready", detail.State)
}

func TestAddArrow_Posts(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusCreated, `{"success":true,"namespace":"github.com/user/a"}`)
	c := newClient(t, srv)

	err := c.AddArrow(context.Background(), "github.com/user/a")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa", rec.path)
}

func TestRemoveArrow_Deletes(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true,"namespace":"github.com/user/a"}`)
	c := newClient(t, srv)

	err := c.RemoveArrow(context.Background(), "github.com/user/a")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.method)
}

func TestGetArrowManifest_ReturnsRaw(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":{"metadata":{"name":"A"}}}`)
	c := newClient(t, srv)

	raw, err := c.GetArrowManifest(context.Background(), "github.com/user/a")
	require.NoError(t, err)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa/manifest", rec.path)
	assert.True(t, json.Valid(raw))
	assert.Contains(t, string(raw), `"name":"A"`)
}

func TestSeedArrowManifest_PostsBody(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusCreated, `{"success":true}`)
	c := newClient(t, srv)

	err := c.SeedArrowManifest(context.Background(), "github.com/user/a", []byte("schema: arrow@v0"))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa/manifest", rec.path)
	assert.Contains(t, rec.body, "schema: arrow@v0")
}

// ─── collections ─────────────────────────────────────────────────────────────

func TestListCollections_DecodesItems(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":[{"namespace":"github.com/user/col","name":"Col","arrow_count":3,"followed":true}]}`)
	c := newClient(t, srv)

	cols, err := c.ListCollections(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/v0/collection", rec.path)
	require.Len(t, cols, 1)
	assert.Equal(t, 3, cols[0].ArrowCount)
}

func TestFollowCollection_Posts(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusCreated, `{"success":true}`)
	c := newClient(t, srv)

	err := c.FollowCollection(context.Background(), "github.com/user/col")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v0/collection/github.com%2Fuser%2Fcol/follow", rec.path)
}

func TestUnfollowCollection_Deletes(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true}`)
	c := newClient(t, srv)

	err := c.UnfollowCollection(context.Background(), "github.com/user/col")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, rec.method)
	assert.Equal(t, "/v0/collection/github.com%2Fuser%2Fcol/follow", rec.path)
}

func TestUpdateCollection_PostsManifest(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true}`)
	c := newClient(t, srv)

	err := c.UpdateCollection(context.Background(), "github.com/user/col")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v0/collection/github.com%2Fuser%2Fcol/manifest", rec.path)
}

func TestGetCollection_DecodesDetail(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":{"namespace":"github.com/user/col","name":"Col","arrows":[],"maintainers":[],"tags":[],"followed":true}}`)
	c := newClient(t, srv)

	col, err := c.GetCollection(context.Background(), "github.com/user/col")
	require.NoError(t, err)
	assert.Equal(t, "/v0/collection/github.com%2Fuser%2Fcol", rec.path)
	assert.True(t, col.Followed)
}

// ─── runtime ─────────────────────────────────────────────────────────────────

func TestExecuteMethod_PostsVariables(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusAccepted, `{"success":true}`)
	c := newClient(t, srv)

	_, err := c.ExecuteMethod(context.Background(), "github.com/user/a", "install", map[string]string{"PORT": "8080"})
	require.NoError(t, err)
	assert.Equal(t, "/v0/runtime/github.com%2Fuser%2Fa/install", rec.path)
	assert.Contains(t, rec.body, `"PORT":"8080"`)
}

func TestExecuteMethod_NoVariablesSendsNoBody(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusAccepted, `{"success":true}`)
	c := newClient(t, srv)

	_, err := c.ExecuteMethod(context.Background(), "github.com/user/a", "stop", nil)
	require.NoError(t, err)
	assert.Empty(t, rec.body)
}

func TestRefreshArrow_PatchesManifest(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true}`)
	c := newClient(t, srv)

	err := c.RefreshArrow(context.Background(), "github.com/user/a")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPatch, rec.method)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa", rec.path)
}

func TestExecuteMethod_StartedTrueOn202(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusAccepted, `{"success":true}`)
	c := newClient(t, srv)

	started, err := c.ExecuteMethod(context.Background(), "github.com/user/a", "install", nil)
	require.NoError(t, err)
	assert.True(t, started, "202 Accepted means work started")
}

func TestExecuteMethod_StartedFalseOn200(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusOK, `{"success":true}`)
	c := newClient(t, srv)

	started, err := c.ExecuteMethod(context.Background(), "github.com/user/a", "install", nil)
	require.NoError(t, err)
	assert.False(t, started, "200 OK means the request was a no-op")
}

func TestGetRuntime_DecodesSnapshot(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":{"namespace":"github.com/user/a","state":"running","active_run":{"method":"_execute","pid":42}}}`)
	c := newClient(t, srv)

	rt, err := c.GetRuntime(context.Background(), "github.com/user/a")
	require.NoError(t, err)
	assert.Equal(t, "/v0/runtime/github.com%2Fuser%2Fa", rec.path)
	assert.Equal(t, "running", rt.State)
	require.NotNil(t, rt.ActiveRun)
	assert.Equal(t, 42, rt.ActiveRun.PID)
}

func TestListRuntimes_DecodesSnapshots(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK,
		`{"success":true,"data":[{"namespace":"github.com/user/a","state":"running"},{"namespace":"github.com/user/b","state":"absent"}]}`)
	c := newClient(t, srv)

	rts, err := c.ListRuntimes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/v0/runtime", rec.path)
	require.Len(t, rts, 2)
	assert.Equal(t, "absent", rts[1].State)
}

// ─── error mapping ───────────────────────────────────────────────────────────

func TestAPIError_NotFoundCarriesMessage(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusNotFound,
		`{"success":false,"error":"not found","namespace":"github.com/user/a"}`)
	c := newClient(t, srv)

	_, err := c.GetArrow(context.Background(), "github.com/user/a")
	require.Error(t, err)

	var apiErr *client.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
	assert.Equal(t, "not found", apiErr.Message)
	assert.Equal(t, "github.com/user/a", apiErr.Namespace)
	assert.Equal(t, 1, client.ExitCode(err))
}

func TestAPIError_NonJSONBodyStillErrors(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusInternalServerError, `boom`)
	c := newClient(t, srv)

	err := c.Health(context.Background())
	require.Error(t, err)
	assert.Equal(t, 1, client.ExitCode(err))
}

func TestExitCode_PlainErrorDefaultsToOne(t *testing.T) {
	assert.Equal(t, 1, client.ExitCode(errors.New("anything")))
}

func TestExitCode_NilIsZero(t *testing.T) {
	assert.Equal(t, 0, client.ExitCode(nil))
}

func TestEnvelope_SuccessFalseWithOKStatusErrors(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusOK, `{"success":false,"error":"drifted"}`)
	c := newClient(t, srv)

	_, err := c.ListArrows(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "drifted"))
}

// ─── coverage: remaining branches ───────────────────────────────────────────

func TestNew_InvalidURIErrors(t *testing.T) {
	_, err := client.New("tcp://bad\x7f uri")
	assert.Error(t, err)
}

func TestNew_UnixHostFormIsAccepted(t *testing.T) {
	// url.Parse of unix://name.sock puts "name.sock" in Host, not Path.
	sock := filepath.Join(t.TempDir(), "q.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	// Force the host form: unix://<first-segment>/<rest>
	c, err := client.New("unix://" + strings.TrimPrefix(sock, "/"))
	require.NoError(t, err)
	assert.NoError(t, c.Health(context.Background()))
}

func TestHealth_UnhealthyStatusErrors(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusOK, `{"status":"degraded"}`)
	c := newClient(t, srv)

	err := c.Health(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "degraded")
}

func TestHealth_ErrorStatusRaw(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusServiceUnavailable, `down`)
	c := newClient(t, srv)

	err := c.Health(context.Background())
	require.Error(t, err)
	var apiErr *client.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
}

func TestHealth_MalformedBodyErrors(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusOK, `not-json`)
	c := newClient(t, srv)

	assert.Error(t, c.Health(context.Background()))
}

func TestValidateArrowManifest_PostsBody(t *testing.T) {
	srv, rec := fakeDaemon(t, http.StatusOK, `{"success":true,"data":{"valid":true}}`)
	c := newClient(t, srv)

	out, err := c.ValidateArrowManifest(context.Background(), "github.com/user/a", []byte("schema: arrow@v0"))
	require.NoError(t, err)
	assert.Equal(t, "/v0/arrow/github.com%2Fuser%2Fa/manifest/validate", rec.path)
	assert.Contains(t, string(out), `"valid":true`)
}

func TestDo_MalformedDataDecodeErrors(t *testing.T) {
	srv, _ := fakeDaemon(t, http.StatusOK, `{"success":true,"data":"not-an-array"}`)
	c := newClient(t, srv)

	_, err := c.ListArrows(context.Background(), nil)
	assert.Error(t, err)
}

func TestAPIError_ErrorWithoutNamespace(t *testing.T) {
	e := &client.APIError{Status: 500, Message: "kaput"}
	assert.Equal(t, "kaput", e.Error())
}

func TestConnError_ErrorAndUnwrap(t *testing.T) {
	inner := errors.New("refused")
	e := &client.ConnError{Server: "http://x", Err: inner}
	assert.Contains(t, e.Error(), "refused")
	assert.True(t, errors.Is(e, inner))
}

func TestExecuteMethod_DaemonDownIsConnError(t *testing.T) {
	c, err := client.New("tcp://127.0.0.1:1")
	require.NoError(t, err)

	_, err = c.ExecuteMethod(context.Background(), "github.com/user/a", "install", map[string]string{"K": "v"})
	assert.Equal(t, 3, client.ExitCode(err))
}

func TestDo_TruncatedBodyReadErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{"succ`)) // fewer bytes than promised
	}))
	t.Cleanup(srv.CloseClientConnections)
	t.Cleanup(srv.Close)
	c := newClient(t, srv)

	_, err := c.ListArrows(context.Background(), nil)
	assert.Error(t, err)
}

func TestDoRaw_TruncatedBodyReadErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte(`{"st`))
	}))
	t.Cleanup(srv.CloseClientConnections)
	t.Cleanup(srv.Close)
	c := newClient(t, srv)

	assert.Error(t, c.Health(context.Background()))
}

func TestAPIError_ErrorWithNamespace(t *testing.T) {
	e := &client.APIError{Status: 404, Message: "not found", Namespace: "github.com/u/r"}
	assert.Equal(t, "not found (github.com/u/r)", e.Error())
}
