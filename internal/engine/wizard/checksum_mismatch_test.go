package wizard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func TestStart_FetchChecksumMismatch_FailedEventCarriesSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	fetch := domainstep.NewFetchStep("fetch", srv.URL, filepath.Join(dir, "asset"), "sha256:"+zeroDigest, "", true)
	req := newTestReq(fetch)
	req.WorkDir = dir

	var failure error
	exec := newTestWizard(t).Start(context.Background(), req)
	for e := range exec.Events() {
		if e.Kind == EventKindStepFailed {
			failure = e.Err
		}
	}

	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, exec.Outcome())
	require.Error(t, failure)
	assert.ErrorIs(t, failure, ErrChecksumMismatch)
}

const zeroDigest = "0000000000000000000000000000000000000000000000000000000000000000"
