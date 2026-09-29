package step_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

func TestRequest_ResolvePath_Table(t *testing.T) {
	workDir := t.TempDir()
	abs := filepath.Join(t.TempDir(), "elsewhere")
	req := wizstep.Request{WorkDir: workDir, Vars: map[string]string{"DIR": "bin"}}

	testCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative path joins the workdir", raw: "dl/tool", want: filepath.Join(workDir, "dl", "tool")},
		{name: "absolute path is kept", raw: abs, want: abs},
		{name: "variables expand before resolving", raw: "${DIR}/tool", want: filepath.Join(workDir, "bin", "tool")},
		{name: "empty path is the workdir", raw: "", want: workDir},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, req.ResolvePath(tc.raw))
		})
	}
}

func TestWithTimeout_Table(t *testing.T) {
	testCases := []struct {
		name         string
		raw          string
		wantErr      bool
		wantDeadline bool
	}{
		{name: "empty raw has no deadline", raw: ""},
		{name: "valid duration sets a deadline", raw: "1h", wantDeadline: true},
		{name: "invalid duration is an error", raw: "soon", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel, err := wizstep.WithTimeout(context.Background(), tc.raw)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer cancel()
			deadline, ok := ctx.Deadline()
			assert.Equal(t, tc.wantDeadline, ok)
			if tc.wantDeadline {
				assert.WithinDuration(t, time.Now().Add(time.Hour), deadline, time.Minute)
			}
		})
	}
}

func TestWithTimeout_CancelStopsContext(t *testing.T) {
	ctx, cancel, err := wizstep.WithTimeout(context.Background(), "")
	require.NoError(t, err)

	cancel()

	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
