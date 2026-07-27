package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
)

// jobGrace is how long a finished job stays readable. The summary is read once,
// after the socket closes, so the job has to outlive its own stream for that
// single read to land.
const jobGrace = 30 * time.Second

// JobStatus is where a discovery pass is: still asking providers, or finished
// and holding its summary until the grace elapses.
type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
)

// Job is one discovery pass as a client sees it. Everything the stream cannot
// carry — the counts and which provider refused — lives here.
type Job struct {
	ID      string
	Query   string
	Status  JobStatus
	Outcome discovery.Outcome
	// ExpiresAt is when the job stops being readable. While the pass is still
	// running it is a floor rather than a promise: the grace is measured from
	// the moment the pass finishes.
	ExpiresAt time.Time
}

// StreamItem is one verified result tagged with the job that produced it. The
// job id is routing metadata for the stream predicate, not a message variant —
// the stream still carries exactly one payload type.
type StreamItem struct {
	JobID  string
	Result discovery.Result
}

// DiscoveryUsecase runs discovery passes as jobs. Start never blocks on the
// pipeline; results reach clients through OnResult and everything else through
// Get.
type DiscoveryUsecase interface {
	Start(
		ctx context.Context,
		text string,
	) (Job, error)

	Get(
		ctx context.Context,
		id string,
	) (*Job, error)

	// Cancel stops a running pass. Nothing is wasted: every result verified so
	// far is already in the vault.
	Cancel(
		ctx context.Context,
		id string,
	)

	// OnResult registers a listener for verified results. Listeners accumulate;
	// registering does not replace the previous one.
	OnResult(
		emit func(StreamItem),
	)
}

// session is one job's mutable state. Everything except id, query and cancel is
// written by the pipeline goroutine, so every field is read under the registry
// lock.
type session struct {
	id        string
	query     string
	cancel    context.CancelFunc
	status    JobStatus
	outcome   discovery.Outcome
	expiresAt time.Time
}

type discoveryUsecase struct {
	pipeline discovery.Discovery
	now      func() time.Time

	mu        sync.Mutex
	sessions  map[string]*session
	listeners []func(StreamItem)
}

// DiscoveryOption configures the registry. It exists so tests can drive the
// grace period from an injected clock instead of waiting for one.
type DiscoveryOption func(*discoveryUsecase)

func WithDiscoveryClock(
	now func() time.Time,
) DiscoveryOption {
	return func(d *discoveryUsecase) {
		if now != nil {
			d.now = now
		}
	}
}

// NewDiscoveryUsecase wraps the discovery pipeline in a job lifecycle. A nil
// pipeline is accepted and reported per request, because a container built
// without a vault or a manifold has no discovery at all and must still serve
// every other route.
func NewDiscoveryUsecase(
	pipeline discovery.Discovery,
	opts ...DiscoveryOption,
) DiscoveryUsecase {
	uc := &discoveryUsecase{
		pipeline: pipeline,
		now:      time.Now,
		sessions: make(map[string]*session),
	}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
}

func (d *discoveryUsecase) Start(
	ctx context.Context,
	text string,
) (Job, error) {
	if d.pipeline == nil {
		return Job{}, fmt.Errorf("discover: discovery is not configured")
	}

	query := strings.TrimSpace(text)
	if query == "" {
		return Job{}, fmt.Errorf("discover: query text is empty")
	}

	// The pass outlives the request that started it: Gin cancels the request
	// context the moment the 202 is written, which would abort the pipeline
	// before it fetched anything.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	now := d.now()
	s := &session{
		id:        uuid.NewString(),
		query:     query,
		cancel:    cancel,
		status:    JobRunning,
		expiresAt: now.Add(jobGrace),
	}

	d.mu.Lock()
	d.sweepLocked(now)
	d.sessions[s.id] = s
	job := jobOfLocked(s)
	d.mu.Unlock()

	go d.run(runCtx, s)

	return job, nil
}

func (d *discoveryUsecase) Get(
	_ context.Context,
	id string,
) (*Job, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sweepLocked(d.now())

	s, ok := d.sessions[id]
	if !ok {
		return nil, fmt.Errorf("discover: job %s: %w", id, apperrors.ErrNotFound)
	}

	job := jobOfLocked(s)
	return &job, nil
}

func (d *discoveryUsecase) Cancel(
	_ context.Context,
	id string,
) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	d.mu.Unlock()

	if !ok {
		return
	}
	s.cancel()
}

func (d *discoveryUsecase) OnResult(
	emit func(StreamItem),
) {
	if emit == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, emit)
}

// run owns the pass. A failed pass still completes the job: a client waiting on
// the summary must never be left reading "running" forever.
func (d *discoveryUsecase) run(
	ctx context.Context,
	s *session,
) {
	defer s.cancel()

	outcome, err := d.pipeline.Discover(ctx, s.query, d.emitter(s.id))
	if err != nil {
		slog.WarnContext(ctx, "discovery: pass failed", "job", s.id, "err", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	s.status = JobCompleted
	s.outcome = outcome
	s.expiresAt = d.now().Add(jobGrace)
}

func (d *discoveryUsecase) emitter(
	id string,
) func(discovery.Result) {
	return func(result discovery.Result) {
		d.mu.Lock()
		listeners := make([]func(StreamItem), len(d.listeners))
		copy(listeners, d.listeners)
		d.mu.Unlock()

		item := StreamItem{JobID: id, Result: result}
		for _, emit := range listeners {
			emit(item)
		}
	}
}

// sweepLocked drops jobs whose grace has elapsed. A running pass is never swept
// however long it takes: the grace measures how long a finished job stays
// readable, not how long a job may live.
func (d *discoveryUsecase) sweepLocked(
	now time.Time,
) {
	for id, s := range d.sessions {
		if s.status == JobCompleted && !now.Before(s.expiresAt) {
			delete(d.sessions, id)
		}
	}
}

func jobOfLocked(
	s *session,
) Job {
	return Job{
		ID:        s.id,
		Query:     s.query,
		Status:    s.status,
		Outcome:   s.outcome,
		ExpiresAt: s.expiresAt,
	}
}
