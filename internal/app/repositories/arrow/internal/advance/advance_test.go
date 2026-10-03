package advance_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowMocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestAdvance_MissingRow_ReturnsNotFound(t *testing.T) {
	m := failOnNetworkManifold(t, nil)
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), &mocks.Vault{}, m)

	err := cat.Advance(context.Background(), rollingNs(), domain.Available{Ref: "nightly-latest", Commit: "c1"})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAdvance_FetchOrCacheFailure_LeavesTheRowUnchanged(t *testing.T) {
	testCases := []struct {
		name      string
		vault     *mocks.Vault
		fetchErr  error
		wantErrIs error
	}{
		{
			name:      "fetch at the target commit fails",
			vault:     &mocks.Vault{},
			fetchErr:  errors.New("503"),
			wantErrIs: apperrors.ErrFetchFailed,
		},
		{
			name:  "the stale cache entry cannot be deleted",
			vault: &mocks.Vault{DeleteArrowErr: errors.New("busy")},
		},
		{
			name:  "the vault cache write fails",
			vault: &mocks.Vault{PutArrowErr: errors.New("disk full")},
		},
		{
			name:      "another identity owns the workdir",
			vault:     &mocks.Vault{PutArrowErr: vault.ErrWorkDirCollision},
			wantErrIs: apperrors.ErrAlreadyExists,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ns := rollingNs()
			axArrow := newTestAsynxArrow(t)
			before := domain.Resolved{Ref: "nightly-latest", Commit: "old111", Fingerprint: "old111"}
			seedSelectorRow(t, axArrow, ns, domain.SelectorPin, before)
			m := &mocks.Manifold{
				ResolveArrowAtCommitResult: &domain.Arrow{Namespace: ns},
				ResolveArrowAtCommitErr:    tc.fetchErr,
			}
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, tc.vault, m)

			err := cat.Advance(context.Background(), ns, domain.Available{Ref: "nightly-latest", Commit: "new222"})

			require.Error(t, err)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
			}
			got, err := axArrow.Get(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, before, got.Resolved)
		})
	}
}

func TestAdvance_EmptyCommit_IsRejectedBeforeAnyFetch(t *testing.T) {
	ns := rollingNs()
	axArrow := newTestAsynxArrow(t)
	seedSelectorRow(t, axArrow, ns, domain.SelectorPin, domain.Resolved{Ref: "nightly-latest", Commit: "old111"})
	v := &mocks.Vault{}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, failOnNetworkManifold(t, nil))

	err := cat.Advance(context.Background(), ns, domain.Available{Ref: "nightly-latest"})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
	assert.Empty(t, v.ArrowOps)
}

func TestAdvance_AsynxFailures_AreMapped(t *testing.T) {
	testCases := []struct {
		name      string
		ax        *arrowMocks.AsynxArrow
		wantErrIs error
	}{
		{
			name: "existence check fails",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return false, errors.New("db down") },
			},
		},
		{
			name: "the advance is rejected",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
				SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, fmt.Errorf("pipeline: %w", asynxModels.ErrValidation)
				},
			},
			wantErrIs: apperrors.ErrStateViolation,
		},
		{
			name: "the advance fails to send",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
				SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, errors.New("closed")
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ns := rollingNs()
			m := &mocks.Manifold{ResolveArrowAtCommitResult: &domain.Arrow{Namespace: ns}}
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, tc.ax, &mocks.Vault{}, m)

			err := cat.Advance(context.Background(), ns, domain.Available{Ref: "nightly-latest", Commit: "c1"})

			require.Error(t, err)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
			}
		})
	}
}

func TestAdopt_AbsentRow_CreatesItOffline(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	axArrow := newTestAsynxArrow(t)
	v := &mocks.Vault{}
	m := failOnNetworkManifold(t, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Quiver"}})
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	resolved := domain.Resolved{Ref: "v1.2.0", Commit: "abc123", Fingerprint: "abc123"}

	require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorChannel, resolved, []byte("manifest"), "ARROW.md"))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
	assert.Equal(t, "Quiver", got.Name)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, resolved, got.Resolved)
	assert.True(t, got.UserInstalled)
	assert.Equal(t, []string{"delete " + ns.String(), "put " + ns.String()}, v.ArrowOps)
	assert.Zero(t, m.ResolveArrowCalls)
}

// The adopted manifest may be named differently from the cached one, so the
// old entry has to go rather than be overwritten under a stale filename.
func TestAdopt_PresentRow_AdvancesItAndReplacesTheCache(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	axArrow := newTestAsynxArrow(t)
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.1.0", Commit: "old111"})
	m := failOnNetworkManifold(t, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Quiver 1.2"}})
	v := &mocks.Vault{}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	resolved := domain.Resolved{Ref: "v1.2.0", Commit: "new222", Fingerprint: "new222"}
	yamlManifest := []byte("metadata:\n  name: Quiver 1.2\n")

	require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorChannel, resolved, yamlManifest, "arrow.yaml"))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, resolved, got.Resolved)
	assert.Equal(t, "Quiver 1.2", got.Name)
	assert.Equal(t, domain.SelectorChannel, got.SelectorKind)
	assert.Equal(t, []string{"delete " + ns.String(), "put " + ns.String()}, v.ArrowOps)
	require.Len(t, v.PutArrowFiles, 1)
	assert.Equal(t, "arrow.yaml", v.PutArrowFiles[0].Filename)
	assert.Equal(t, yamlManifest, v.PutArrowFiles[0].Content)
}

// A self-update that rolls back leaves the row advanced to a release the old
// build never became: the old build's boot adopts its own state and brings the
// row back, so the settler needs no special case for core.
func TestAdopt_RowAdvancedByARolledBackUpdate_IsBroughtBackToTheRunningBuild(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	axArrow := newTestAsynxArrow(t)
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.2.0", Commit: "new222", Fingerprint: "new222"})
	m := failOnNetworkManifold(t, adoptedManifest("Quiver"))
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, m)
	running := domain.Resolved{Ref: "v1.1.0", Commit: "old111", Fingerprint: "old111"}

	require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorChannel, running, []byte("manifest"), "ARROW.md"))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, running, got.Resolved)
}

// Adopt runs on every core boot, so an adopt that changes nothing must write
// nothing: no event and no cache churn.
func TestAdopt_UnchangedResolvedAndManifest_WritesNothing(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	axArrow := newTestAsynxArrow(t)
	resolved := domain.Resolved{Ref: "v1.2.0", Commit: "abc123", Fingerprint: "abc123"}
	m := failOnNetworkManifold(t, adoptedManifest("Quiver"))
	first := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, m)
	require.NoError(t, first.Adopt(context.Background(), ns, domain.SelectorChannel, resolved, []byte("manifest"), "ARROW.md"))

	var sent []string
	ax := &arrowMocks.AsynxArrow{
		ExistsFn: func(ctx context.Context, id string) (bool, error) { return axArrow.Exists(ctx, id) },
		GetFn:    func(ctx context.Context, id string) (domain.Arrow, error) { return axArrow.Get(ctx, id) },
		SendWaitFn: func(_ context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			sent = append(sent, cmd.EventName())
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	v := &mocks.Vault{}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, v, failOnNetworkManifold(t, adoptedManifest("Quiver")))

	require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorChannel, resolved, []byte("manifest"), "ARROW.md"))

	assert.Empty(t, sent)
	assert.Empty(t, v.ArrowOps)
}

// A seed or a collection-local adopt names its ref without a commit. On a
// row that already learned the commit of that same ref (from Add or an
// update) it is no advance: the commit stays, so the row is not offered the
// update it already ran, and only the manifest follows the seeded bytes.
func TestAdopt_CommitlessSameRef_KeepsTheLearnedCommit(t *testing.T) {
	testCases := []struct {
		name     string
		manifest string
		wantName string
		wantSent []string
	}{
		{name: "new bytes refresh the manifest", manifest: "Seeded", wantName: "Seeded", wantSent: []string{"arrow.manifest_refreshed."}},
		{name: "same bytes write nothing", manifest: "Installed", wantName: "Installed"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ns := domain.Namespace("github.com/user/pkg@v1.0.0")
			learned := domain.Resolved{Ref: "v1.0.0", Commit: "c100", Fingerprint: "c100"}
			axArrow := newTestAsynxArrow(t)
			seed := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, failOnNetworkManifold(t, adoptedManifest("Installed")))
			require.NoError(t, seed.Adopt(context.Background(), ns, domain.SelectorPin, learned, []byte("installed"), "ARROW.md"))
			var sent []string
			ax := &arrowMocks.AsynxArrow{
				ExistsFn: func(ctx context.Context, id string) (bool, error) { return axArrow.Exists(ctx, id) },
				GetFn:    func(ctx context.Context, id string) (domain.Arrow, error) { return axArrow.Get(ctx, id) },
				SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					sent = append(sent, strings.TrimSuffix(cmd.EventName(), ns.String()))
					return axArrow.SendWait(ctx, cmd)
				},
			}
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{}, failOnNetworkManifold(t, adoptedManifest(tc.manifest)))

			require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorPin, domain.Resolved{Ref: "v1.0.0"}, []byte(tc.manifest), "ARROW.md"))

			got, err := axArrow.Get(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, got.Name)
			assert.Equal(t, learned, got.Resolved)
			assert.Equal(t, tc.wantSent, sent)
		})
	}
}

// A commit-less adopt of a different ref is still an advance.
func TestAdopt_CommitlessOtherRef_Advances(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	axArrow := newTestAsynxArrow(t)
	seedSelectorRow(t, axArrow, ns, domain.SelectorPin, domain.Resolved{Ref: "v0.9.0", Commit: "c090", Fingerprint: "c090"})
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, failOnNetworkManifold(t, adoptedManifest("Seeded")))

	require.NoError(t, cat.Adopt(context.Background(), ns, domain.SelectorPin, domain.Resolved{Ref: "v1.0.0"}, []byte("seeded"), "ARROW.md"))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Seeded", got.Name)
	assert.Equal(t, domain.Resolved{Ref: "v1.0.0"}, got.Resolved)
}

func TestAdopt_RefreshRejected_IsMapped(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	resolved := domain.Resolved{Ref: "v1.0.0"}
	var sent []string
	ax := &arrowMocks.AsynxArrow{
		ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
		GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
			return domain.Arrow{Namespace: ns, Resolved: resolved, UserInstalled: true}, nil
		},
		SendWaitFn: func(_ context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			sent = append(sent, cmd.EventName())
			return asynxModels.Event[domain.Arrow]{}, fmt.Errorf("pipeline: %w", asynxModels.ErrValidation)
		},
	}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{}, failOnNetworkManifold(t, adoptedManifest("New")))

	err := cat.Adopt(context.Background(), ns, domain.SelectorPin, resolved, []byte("new"), "ARROW.md")

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
	assert.Equal(t, []string{"arrow.manifest_refreshed." + ns.String()}, sent)
}

func TestAdopt_Failures(t *testing.T) {
	validNs := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	testCases := []struct {
		name      string
		ns        domain.Namespace
		filename  string
		manifold  *mocks.Manifold
		vault     *mocks.Vault
		ax        *arrowMocks.AsynxArrow
		wantErrIs error
	}{
		{
			name:      "manifest without a filename",
			ns:        validNs,
			manifold:  &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidManifest,
		},
		{
			name:     "the stale cache entry cannot be deleted",
			ns:       validNs,
			filename: "ARROW.md",
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{DeleteArrowErr: errors.New("busy")},
		},
		{
			name:      "invalid namespace",
			filename:  "ARROW.md",
			ns:        domain.Namespace("not a namespace"),
			manifold:  &mocks.Manifold{},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidNamespace,
		},
		{
			name:      "namespace without a selector",
			filename:  "ARROW.md",
			ns:        domain.Namespace("github.com/rabbytesoftware/quiver.core"),
			manifold:  &mocks.Manifold{},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidNamespace,
		},
		{
			name:      "selector with an empty ref component",
			filename:  "ARROW.md",
			ns:        domain.Namespace("github.com/rabbytesoftware/quiver.core@feat//x"),
			manifold:  &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidNamespace,
		},
		{
			name:      "selector with a trailing slash",
			filename:  "ARROW.md",
			ns:        domain.Namespace("github.com/rabbytesoftware/quiver.core@feat/"),
			manifold:  &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidNamespace,
		},
		{
			name:      "manifest does not parse",
			filename:  "ARROW.md",
			ns:        validNs,
			manifold:  &mocks.Manifold{ParseArrowErr: errors.New("bad yaml")},
			vault:     &mocks.Vault{},
			wantErrIs: apperrors.ErrInvalidManifest,
		},
		{
			name:     "vault write fails",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{PutArrowErr: errors.New("disk full")},
		},
		{
			name:     "existence check fails",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return false, errors.New("db down") },
			},
		},
		{
			name:     "reading the present row fails",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
				GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
					return domain.Arrow{}, errors.New("db down")
				},
			},
		},
		{
			name:     "the present row is forgotten before it is read",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
				GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
					return domain.Arrow{}, fmt.Errorf("get: %w", asynxModels.ErrNotFound)
				},
			},
			wantErrIs: apperrors.ErrNotFound,
		},
		{
			name:     "the create loses the race to another create",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
				SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, fmt.Errorf("pipeline: %w", asynxModels.ErrValidation)
				},
			},
			wantErrIs: apperrors.ErrAlreadyExists,
		},
		{
			name:     "the create conflicts with another append",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
				SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, fmt.Errorf("pipeline: %w", asynxModels.ErrPipelineFailed)
				},
			},
			wantErrIs: apperrors.ErrAlreadyExists,
		},
		{
			name:     "the create fails to send",
			filename: "ARROW.md",
			ns:       validNs,
			manifold: &mocks.Manifold{ParseArrowResult: &domain.Arrow{}},
			vault:    &mocks.Vault{},
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
				SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, errors.New("closed")
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxArrow(t)
			if tc.ax != nil {
				ax = tc.ax
			}
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, tc.vault, tc.manifold)

			err := cat.Adopt(context.Background(), tc.ns, domain.SelectorChannel,
				domain.Resolved{Ref: "v1.2.0", Commit: "abc123"}, []byte("manifest"), tc.filename)

			require.Error(t, err)
			if tc.wantErrIs != nil {
				assert.ErrorIs(t, err, tc.wantErrIs)
			}
		})
	}
}

func TestAdopt_RepairRejected_IsMapped(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	resolved := domain.Resolved{Ref: "v1.2.0", Commit: "abc123"}
	ax := &arrowMocks.AsynxArrow{
		ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
		GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
			return domain.Arrow{Namespace: ns, Resolved: resolved}, nil
		},
		SendWaitFn: func(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, fmt.Errorf("pipeline: %w", asynxModels.ErrValidation)
		},
	}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{}, &mocks.Manifold{ParseArrowResult: &domain.Arrow{}})

	err := cat.Adopt(context.Background(), ns, domain.SelectorChannel, resolved, []byte("manifest"), "ARROW.md")

	require.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestAdopt_AdvanceFailure_SkipsTheRepair(t *testing.T) {
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.core@stable")
	var sent []string
	ax := &arrowMocks.AsynxArrow{
		ExistsFn: func(_ context.Context, _ string) (bool, error) { return true, nil },
		GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
			return domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "v1.1.0"}}, nil
		},
		SendWaitFn: func(_ context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			sent = append(sent, cmd.EventName())
			return asynxModels.Event[domain.Arrow]{}, errors.New("db down")
		},
	}
	cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{}, &mocks.Manifold{ParseArrowResult: &domain.Arrow{}})

	err := cat.Adopt(context.Background(), ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.2.0", Commit: "abc"}, []byte("manifest"), "ARROW.md")

	require.Error(t, err)
	assert.Equal(t, []string{"arrow.advanced." + ns.String()}, sent)
}

// A remote that cannot be read is a gateway failure, the same as for Add.
func TestAdoptInstalled_UnclassifiedResolveFailure_IsAFetchFailure(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveAdoptionFn: func(context.Context, domain.Namespace, string) (arrowstore.Adoption, error) {
			return arrowstore.Adoption{}, errors.New("connection reset")
		},
	}
	cat := newTestable(r, newTestAsynxArrow(t), &mocks.Vault{}, &mocks.Manifold{})

	err := cat.AdoptInstalled(context.Background(), adoptBare.WithRef("stable"), "v1.2.0")

	require.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestAdoptInstalled_ManifestThatDoesNotParse_IsAnInvalidManifest(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveAdoptionFn: func(_ context.Context, ns domain.Namespace, ref string) (arrowstore.Adoption, error) {
			return arrowstore.Adoption{
				Identity: ns,
				Kind:     domain.SelectorChannel,
				Resolved: domain.Resolved{Ref: ref, Commit: "c120"},
				Manifest: []byte("not a manifest"),
				Filename: "ARROW.md",
			}, nil
		},
	}
	m := &mocks.Manifold{ParseArrowErr: errors.New("bad yaml")}
	cat := newTestable(r, newTestAsynxArrow(t), &mocks.Vault{}, m)

	err := cat.AdoptInstalled(context.Background(), adoptBare.WithRef("stable"), "v1.2.0")

	require.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}
