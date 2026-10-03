package activation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/activation"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type recorder struct {
	got domainRuntime.PendingActivation
	ns  domain.Namespace
	err error
}

func (r *recorder) RecordPendingActivation(_ context.Context, ns domain.Namespace, p domainRuntime.PendingActivation) error {
	r.ns, r.got = ns, p
	return r.err
}

func endedUpdate(workdir string) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref: "github.com/rabbytesoftware/quiver.core@nightly-latest",
		LastReturn: &domainRuntime.Return{
			Method:    domain.MethodUpdate,
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: map[string]string{domain.VarWorkdir: workdir, domain.VarRef: "nightly-2"},
		},
	}
}

func fixedNow() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }

func TestStager_Stage_RecordsTheVerifiedBinary(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "quiver-new"), []byte("new build"), 0o755))
	rec := &recorder{}
	stager := activation.NewStager(rec, "quiver-new", activation.WithClock(fixedNow))

	err := stager.Stage(context.Background(), endedUpdate(workdir), domain.Available{Ref: "nightly-2", Commit: "c2"})

	require.NoError(t, err)
	assert.Equal(t, endedUpdate("").Ref, rec.ns)
	assert.Equal(t, "nightly-2", rec.got.Version)
	assert.Equal(t, "c2", rec.got.Commit)
	assert.Equal(t, filepath.Join(workdir, "quiver-new"), rec.got.Path)
	assert.EqualValues(t, len("new build"), rec.got.Size)
	assert.Len(t, rec.got.Digest, 64)
	assert.Equal(t, fixedNow(), rec.got.StagedAt)
	assert.False(t, rec.got.Activating)
}

func TestStager_Stage_FallsBackToTheRunsRefWhenNoTargetWasRemembered(t *testing.T) {
	workdir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "quiver-new"), []byte("new build"), 0o755))
	rec := &recorder{}

	err := activation.NewStager(rec, "quiver-new").Stage(context.Background(), endedUpdate(workdir), domain.Available{})

	require.NoError(t, err)
	assert.Equal(t, "nightly-2", rec.got.Version)
	assert.Empty(t, rec.got.Commit)
}

func TestStager_Stage_Failures(t *testing.T) {
	boom := errors.New("event store down")
	emptyDir := t.TempDir()
	emptyFileDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(emptyFileDir, "quiver-new"), nil, 0o755))
	goodDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(goodDir, "quiver-new"), []byte("x"), 0o755))

	testCases := []struct {
		name    string
		ended   domainRuntime.ArrowRuntime
		target  domain.Available
		record  error
		wantErr error
	}{
		{name: "no return", ended: domainRuntime.ArrowRuntime{Ref: "x@y"}},
		{name: "no workdir", ended: endedUpdate("")},
		{name: "binary missing", ended: endedUpdate(emptyDir)},
		{name: "binary empty", ended: endedUpdate(emptyFileDir)},
		{name: "no version to name it by", ended: func() domainRuntime.ArrowRuntime {
			rt := endedUpdate(goodDir)
			rt.LastReturn.Variables = map[string]string{domain.VarWorkdir: goodDir}
			return rt
		}()},
		{name: "recording fails", ended: endedUpdate(goodDir), record: boom, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{err: tc.record}

			err := activation.NewStager(rec, "quiver-new").Stage(context.Background(), tc.ended, tc.target)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}
