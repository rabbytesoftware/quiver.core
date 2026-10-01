package arrows_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const adoptPath = encodedNS + "@stable/adopt"

func postAdopt(t *testing.T, svc *mocks.ArrowService, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func assertFailure(t *testing.T, body []byte) {
	t.Helper()
	var env struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.False(t, env.Success)
}

func TestAdoptInstalled_Adopted_Returns201(t *testing.T) {
	testCases := []struct {
		name    string
		path    string
		body    string
		wantNs  domain.Namespace
		wantRef string
	}{
		{
			name: "selector from the path", path: adoptPath, body: `{"resolved_ref":"v1.2.0"}`,
			wantNs: "github.com/user/repo@stable", wantRef: "v1.2.0",
		},
		{
			name: "refless namespace stays refless", path: encodedNS + "/adopt", body: `{"resolved_ref":"v1.2.0"}`,
			wantNs: "github.com/user/repo", wantRef: "v1.2.0",
		},
		{
			name: "unknown fields are ignored", path: adoptPath, body: `{"resolved_ref":"v1.2.0","channel":"beta"}`,
			wantNs: "github.com/user/repo@stable", wantRef: "v1.2.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.ArrowService{}

			w := postAdopt(t, svc, tc.path, tc.body)

			assert.Equal(t, http.StatusCreated, w.Code)
			assertSuccess(t, w.Body.Bytes())
			assert.Equal(t, []mocks.AdoptInstalledCall{{Namespace: tc.wantNs, ResolvedRef: tc.wantRef}}, svc.AdoptInstalledCalls)
		})
	}
}

func TestAdoptInstalled_ServiceErrors(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "ref the selector could never resolve to", err: apperrors.ErrInvalidNamespace, wantStatus: http.StatusBadRequest},
		{name: "ref the remote does not hold", err: apperrors.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "manifest at the ref does not parse", err: apperrors.ErrInvalidManifest, wantStatus: http.StatusUnprocessableEntity},
		{name: "row write rejected", err: apperrors.ErrStateViolation, wantStatus: http.StatusUnprocessableEntity},
		{name: "remote unreachable", err: apperrors.ErrFetchFailed, wantStatus: http.StatusBadGateway},
		{name: "unclassified", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.ArrowService{AdoptInstalledErr: tc.err}

			w := postAdopt(t, svc, adoptPath, `{"resolved_ref":"v1.2.0"}`)

			assert.Equal(t, tc.wantStatus, w.Code)
			assertFailure(t, w.Body.Bytes())
		})
	}
}

func TestAdoptInstalled_BadRequests_NeverReachTheService(t *testing.T) {
	testCases := []struct {
		name string
		path string
		body string
	}{
		{name: "no body", path: adoptPath, body: ""},
		{name: "not json", path: adoptPath, body: "resolved_ref=v1.2.0"},
		{name: "not an object", path: adoptPath, body: `["v1.2.0"]`},
		{name: "wrong type", path: adoptPath, body: `{"resolved_ref":12}`},
		{name: "missing resolved ref", path: adoptPath, body: `{}`},
		{name: "blank resolved ref", path: adoptPath, body: `{"resolved_ref":"  "}`},
		{name: "single segment namespace", path: "/v0/arrow/notanamespace/adopt", body: `{"resolved_ref":"v1.2.0"}`},
		{name: "empty segment namespace", path: "/v0/arrow/github.com%2F%2Frepo/adopt", body: `{"resolved_ref":"v1.2.0"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.ArrowService{}

			w := postAdopt(t, svc, tc.path, tc.body)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assertFailure(t, w.Body.Bytes())
			assert.Empty(t, svc.AdoptInstalledCalls)
		})
	}
}
