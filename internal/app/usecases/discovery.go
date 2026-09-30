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

// subscriberGrace is how long a running pass survives with nobody attached to
// its stream. It bridges a reconnect: a client that drops and redials inside it
// resumes the same pass and is replayed what it missed instead of restarting it.
const subscriberGrace = 5 * time.Second

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
	JobID string
	// Seq numbers the job's items from 1 in emission order. It is what lets a
	// late subscriber's replay and the live feed be stitched without a
	// duplicate or a gap.
	Seq    uint64
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

	// Attach records a subscriber on the job's stream. A pass with no
	// subscriber is cancelled once subscriberGrace elapses; attaching inside
	// the grace keeps it running.
	Attach(
		id string,
	)

	// Detach is Attach's counterpart. The pass is cancelled only when the last
	// subscriber has left and the grace has run out.
	Detach(
		id string,
	)

	// Replay returns every result the job has emitted so far, in order, so a
	// subscriber that connected after the pass began still receives them.
	Replay(
		id string,
	) []StreamItem

	// Done is closed when the job has finished. It is already closed for a job
	// that is finished or unknown, so a subscriber never waits on a pass that
	// can no longer emit.
	Done(
		id string,
	) <-chan struct{}

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

	items       []StreamItem
	done        chan struct{}
	subscribers int
	idle        *time.Timer
}

type discoveryUsecase struct {
	pipeline discovery.Discovery
	now      func() time.Time
	grace    time.Duration

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

// WithDiscoverySubscriberGrace overrides how long a pass outlives its last
// subscriber, so tests do not wait out the production value.
func WithDiscoverySubscriberGrace(
	grace time.Duration,
) DiscoveryOption {
	return func(d *discoveryUsecase) {
		d.grace = grace
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
		grace:    subscriberGrace,
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
		done:      make(chan struct{}),
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

func (d *discoveryUsecase) Attach(
	id string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.sessions[id]
	if !ok {
		return
	}
	s.subscribers++
	if s.idle != nil {
		s.idle.Stop()
		s.idle = nil
	}
}

func (d *discoveryUsecase) Detach(
	id string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.sessions[id]
	if !ok || s.subscribers == 0 {
		return
	}
	s.subscribers--
	if s.subscribers > 0 || s.status != JobRunning {
		return
	}
	s.idle = time.AfterFunc(d.grace, func() { d.cancelIfAbandoned(s) })
}

func (d *discoveryUsecase) cancelIfAbandoned(
	s *session,
) {
	d.mu.Lock()
	abandoned := s.subscribers == 0 && s.status == JobRunning
	d.mu.Unlock()

	if abandoned {
		s.cancel()
	}
}

func (d *discoveryUsecase) Replay(
	id string,
) []StreamItem {
	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.sessions[id]
	if !ok {
		return nil
	}
	return append([]StreamItem(nil), s.items...)
}

func (d *discoveryUsecase) Done(
	id string,
) <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	if s, ok := d.sessions[id]; ok {
		return s.done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
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

	outcome, err := d.pipeline.Discover(ctx, s.query, d.emitter(s))
	if err != nil {
		slog.WarnContext(ctx, "discovery: pass failed", "job", s.id, "err", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	s.status = JobCompleted
	s.outcome = outcome
	s.expiresAt = d.now().Add(jobGrace)
	if s.idle != nil {
		s.idle.Stop()
	}
	close(s.done)
}

func (d *discoveryUsecase) emitter(
	s *session,
) func(discovery.Result) {
	return func(result discovery.Result) {
		d.mu.Lock()
		item := StreamItem{
			JobID:  s.id,
			Seq:    uint64(len(s.items)) + 1,
			Result: result,
		}
		s.items = append(s.items, item)
		listeners := make([]func(StreamItem), len(d.listeners))
		copy(listeners, d.listeners)
		d.mu.Unlock()

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
