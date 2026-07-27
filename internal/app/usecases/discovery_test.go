package usecases

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// discoverFn is the shape of discovery.Discovery's only method, so a test can
// supply one as a closure.
type discoverFn func(
	ctx context.Context,
	text string,
	emit func(discovery.Result),
) (discovery.Outcome, error)

type stubPipeline struct {
	fn discoverFn
}

func (s *stubPipeline) Discover(
	ctx context.Context,
	text string,
	emit func(discovery.Result),
) (discovery.Outcome, error) {
	if s.fn == nil {
		return discovery.Outcome{}, nil
	}
	return s.fn(ctx, text, emit)
}

// testClock is read by the usecase goroutine and written by the test, so it
// carries its own lock rather than relying on the usecase's.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// waitCompleted polls the registry rather than sleeping: the pipeline goroutine
// marks the job completed after Discover returns, and nothing else observes it.
func waitCompleted(
	t *testing.T,
	uc DiscoveryUsecase,
	id string,
) Job {
	t.Helper()

	var job Job
	require.Eventually(t, func() bool {
		got, err := uc.Get(context.Background(), id)
		if err != nil || got.Status != JobCompleted {
			return false
		}
		job = *got
		return true
	}, 2*time.Second, time.Millisecond)
	return job
}

func TestDiscovery_Start_ReturnsRunningJobImmediately(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		close(entered)
		<-release
		return discovery.Outcome{}, nil
	}})

	job, err := uc.Start(context.Background(), "  chrom  ")
	require.NoError(t, err)
	assert.Equal(t, JobRunning, job.Status)
	assert.Equal(t, "chrom", job.Query)
	assert.NotEmpty(t, job.ID)
	assert.False(t, job.ExpiresAt.IsZero())

	// The pipeline is inside Discover and still blocked, so Start cannot have
	// waited for it.
	<-entered
	running, err := uc.Get(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobRunning, running.Status)

	close(release)
	waitCompleted(t, uc, job.ID)
}

func TestDiscovery_Start_BlankQueryReturnsError(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	_, err := uc.Start(context.Background(), "   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query text is empty")
}

func TestDiscovery_Start_NilPipelineReturnsError(t *testing.T) {
	uc := NewDiscoveryUsecase(nil)

	_, err := uc.Start(context.Background(), "chrom")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestDiscovery_TwoStartsGetDistinctIDs(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	first, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	second, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)

	assert.NotEqual(t, first.ID, second.ID)
	waitCompleted(t, uc, first.ID)
	waitCompleted(t, uc, second.ID)
}

// TestDiscovery_Start_OutlivesTheRequestContext guards the surprise that makes
// this asynchronous at all: Gin cancels the request context the moment the 202
// is written, which would kill the pipeline before it fetched anything.
func TestDiscovery_Start_OutlivesTheRequestContext(t *testing.T) {
	observed := make(chan error, 1)
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		ctx context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		observed <- ctx.Err()
		return discovery.Outcome{}, nil
	}})

	reqCtx, cancel := context.WithCancel(context.Background())
	job, err := uc.Start(reqCtx, "chrom")
	require.NoError(t, err)
	cancel()

	require.NoError(t, <-observed)
	waitCompleted(t, uc, job.ID)
}

func TestDiscovery_Get_UnknownIDReturnsNotFound(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	got, err := uc.Get(context.Background(), "missing")
	assert.Nil(t, got)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestDiscovery_CompletedJobCarriesOutcome(t *testing.T) {
	want := discovery.Outcome{
		Found:    3,
		Verified: 2,
		Skipped:  1,
		Providers: []discovery.ProviderOutcome{
			{Host: "github.com", OK: true, Returned: 3},
			{Host: "gitlab.com", Reason: discovery.ReasonRateLimited, RetryAfter: 40 * time.Second},
		},
	}
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		return want, nil
	}})

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)

	job := waitCompleted(t, uc, started.ID)
	assert.Equal(t, want, job.Outcome)
}

// TestDiscovery_FailedPassStillCompletes: a pass that errors leaves a readable
// job rather than a job stuck running forever.
func TestDiscovery_FailedPassStillCompletes(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		return discovery.Outcome{}, assert.AnError
	}})

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)

	job := waitCompleted(t, uc, started.ID)
	assert.Equal(t, discovery.Outcome{}, job.Outcome)
}

func TestDiscovery_CompletedJobReadableDuringGrace(t *testing.T) {
	clock := newTestClock()
	uc := NewDiscoveryUsecase(&stubPipeline{}, WithDiscoveryClock(clock.Now))

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, started.ID)

	clock.advance(jobGrace - time.Second)

	job, err := uc.Get(context.Background(), started.ID)
	require.NoError(t, err)
	assert.Equal(t, JobCompleted, job.Status)
}

func TestDiscovery_JobEvictedAfterGrace(t *testing.T) {
	clock := newTestClock()
	uc := NewDiscoveryUsecase(&stubPipeline{}, WithDiscoveryClock(clock.Now))

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, started.ID)

	clock.advance(jobGrace + time.Second)

	got, err := uc.Get(context.Background(), started.ID)
	assert.Nil(t, got)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// TestDiscovery_RunningJobSurvivesTheGrace: the grace measures how long a
// finished job stays readable, so a slow pass must not be evicted underneath
// its own subscriber.
func TestDiscovery_RunningJobSurvivesTheGrace(t *testing.T) {
	clock := newTestClock()
	entered := make(chan struct{})
	release := make(chan struct{})
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		close(entered)
		<-release
		return discovery.Outcome{}, nil
	}}, WithDiscoveryClock(clock.Now))

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	<-entered

	clock.advance(jobGrace * 10)

	job, err := uc.Get(context.Background(), started.ID)
	require.NoError(t, err)
	assert.Equal(t, JobRunning, job.Status)

	close(release)
	waitCompleted(t, uc, started.ID)
}

// TestDiscovery_StartSweepsExpiredJobs proves eviction is not merely hidden by
// the read path: the map itself drops the entry.
func TestDiscovery_StartSweepsExpiredJobs(t *testing.T) {
	clock := newTestClock()
	uc := NewDiscoveryUsecase(&stubPipeline{}, WithDiscoveryClock(clock.Now))

	first, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, first.ID)

	clock.advance(jobGrace + time.Second)

	second, err := uc.Start(context.Background(), "firef")
	require.NoError(t, err)
	waitCompleted(t, uc, second.ID)

	registry, ok := uc.(*discoveryUsecase)
	require.True(t, ok)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	assert.Len(t, registry.sessions, 1)
	assert.Contains(t, registry.sessions, second.ID)
}

func TestDiscovery_CancelStopsThePipeline(t *testing.T) {
	entered := make(chan struct{})
	observed := make(chan error, 1)
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		ctx context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		close(entered)
		<-ctx.Done()
		observed <- ctx.Err()
		return discovery.Outcome{Verified: 1}, nil
	}})

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	<-entered

	uc.Cancel(context.Background(), started.ID)

	require.ErrorIs(t, <-observed, context.Canceled)

	// Cancelling banks the work done so far rather than discarding the job.
	job := waitCompleted(t, uc, started.ID)
	assert.Equal(t, 1, job.Outcome.Verified)
}

func TestDiscovery_CancelUnknownIDIsNoOp(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	assert.NotPanics(t, func() { uc.Cancel(context.Background(), "missing") })
}

func TestDiscovery_ResultsEmittedThroughCallback(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/one"), Stars: 7})
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/two")})
		return discovery.Outcome{Verified: 2}, nil
	}})

	items := make(chan StreamItem, 4)
	uc.OnResult(func(item StreamItem) { items <- item })

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, started.ID)
	close(items)

	got := make([]StreamItem, 0, 2)
	for item := range items {
		got = append(got, item)
	}
	require.Len(t, got, 2)
	assert.Equal(t, started.ID, got[0].JobID)
	assert.Equal(t, domain.Namespace("github.com/user/one"), got[0].Result.Namespace)
	assert.Equal(t, 7, got[0].Result.Stars)
	assert.Equal(t, started.ID, got[1].JobID)
}

// TestDiscovery_OnResult_EveryListenerSees registers twice, because the hook
// appends listeners rather than replacing the last one.
func TestDiscovery_OnResult_EveryListenerSees(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/one")})
		return discovery.Outcome{}, nil
	}})

	first := make(chan StreamItem, 1)
	second := make(chan StreamItem, 1)
	uc.OnResult(func(item StreamItem) { first <- item })
	uc.OnResult(func(item StreamItem) { second <- item })

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, started.ID)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
}

func TestDiscovery_OnResult_NilListenerIgnored(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/one")})
		return discovery.Outcome{}, nil
	}})
	uc.OnResult(nil)

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, started.ID)
}

func TestContainerNew_WithoutDiscovery_LeavesUsecaseNil(t *testing.T) {
	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    &ucmocks.MockRuntime{},
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}

	c, err := New(repos, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, c.Discovery)
}

func TestContainerNew_WithDiscovery_BuildsUsecase(t *testing.T) {
	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    &ucmocks.MockRuntime{},
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
		Discovery:  &stubPipeline{},
	}

	c, err := New(repos, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, c.Discovery)

	job, err := c.Discovery.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, c.Discovery, job.ID)
}

func TestDiscovery_ClientNeverConnects_PipelineStillCompletes(t *testing.T) {
	emitted := make(chan struct{})
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		defer close(emitted)
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/one")})
		return discovery.Outcome{Found: 1, Verified: 1}, nil
	}})

	started, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	<-emitted

	job := waitCompleted(t, uc, started.ID)
	assert.Equal(t, 1, job.Outcome.Verified)
}

func TestDiscovery_ConcurrentStartsAreRaceFree(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		emit(discovery.Result{Namespace: domain.Namespace("github.com/user/one")})
		return discovery.Outcome{Verified: 1}, nil
	}})
	uc.OnResult(func(StreamItem) {})

	const starts = 32

	var wg sync.WaitGroup
	ids := make([]string, starts)
	for i := range starts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := uc.Start(context.Background(), "chrom")
			assert.NoError(t, err)
			ids[i] = job.ID
		}()
	}
	wg.Wait()

	unique := make(map[string]struct{}, starts)
	for _, id := range ids {
		unique[id] = struct{}{}
		waitCompleted(t, uc, id)
	}
	assert.Len(t, unique, starts)
}
