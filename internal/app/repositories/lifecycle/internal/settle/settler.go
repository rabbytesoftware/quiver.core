package settle

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const (
	// commitTimeout bounds closing an update bracket: one live ref listing
	// and one manifest fetch, each already bounded by the manifold's own
	// fetch timeout.
	commitTimeout = 2 * time.Minute
	// restoreTimeout bounds putting the installed manifest back after a
	// bracket failed, which must happen even when the caller gave up.
	restoreTimeout = 30 * time.Second
)

// Settler closes update brackets: it commits a succeeded update, restores
// the installed manifest otherwise, and re-derives the row's badge.
type Settler interface {
	// Settling reports whether an update of ns began and has not committed
	// yet. Its runtime may already read ready or outdated: the commit that
	// advances the row runs after the update's steps ended.
	Settling(ns domain.Namespace) bool
	// HoldBadge tells a version check whether to leave ns's badge alone:
	// while ns settles, the row still names the target about to be stamped.
	// A held check is remembered, and the settle re-derives the badge for it.
	HoldBadge(ns domain.Namespace) bool
	OnUpdateEnded(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)
	// RestoreAbandoned puts the installed release's manifest back on a row
	// whose bracket staged a target and then failed before any run began.
	// The restore is tracked like a commit, so a shutdown drain waits for
	// it, aborts it, or refuses it.
	RestoreAbandoned(
		ctx context.Context,
		ns domain.Namespace,
	)
}

type settler struct {
	arrow   Arrow
	runtime Runtime
	targets Targets
	commits Commits
	holds   *badgeHolds
	detach  func(fn func())
	// commitTimeout bounds a detached update commit, which has no caller
	// whose context would end it.
	commitTimeout time.Duration
}

// Option configures New.
type Option func(*settler)

// WithDetach replaces how a settling is run off the delivering goroutine.
func WithDetach(
	detach func(fn func()),
) Option {
	return func(s *settler) { s.detach = detach }
}

// WithCommitTimeout replaces the bound of a detached update commit.
func WithCommitTimeout(
	timeout time.Duration,
) Option {
	return func(s *settler) { s.commitTimeout = timeout }
}

func New(
	arrow Arrow,
	runtime Runtime,
	targets Targets,
	commits Commits,
	opts ...Option,
) Settler {
	s := &settler{
		arrow:         arrow,
		runtime:       runtime,
		targets:       targets,
		commits:       commits,
		holds:         &badgeHolds{dirty: map[domain.Namespace]bool{}},
		detach:        func(fn func()) { go fn() },
		commitTimeout: commitTimeout,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Settling reads the remembered target before the running commits:
// OnUpdateEnded registers the commit before it releases the target, so in
// this order no read falls between the two.
func (s *settler) Settling(ns domain.Namespace) bool {
	return s.targets.Pending(ns) || s.commits.InFlight(ns)
}

func (s *settler) HoldBadge(ns domain.Namespace) bool {
	s.holds.mu.Lock()
	defer s.holds.mu.Unlock()
	if !s.Settling(ns) {
		return false
	}
	s.holds.dirty[ns] = true
	return true
}

func (s *settler) RestoreAbandoned(
	ctx context.Context,
	ns domain.Namespace,
) {
	if !s.commits.Begin(ns) {
		return
	}
	restoreCtx, cancel := s.commits.Bound(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	defer s.releaseSettled(restoreCtx, ns)
	s.restoreInstalled(restoreCtx, ns)
}

// OnUpdateEnded closes the update bracket: it always releases the row's
// remembered target, so the next update is admitted, and settles the row:
// it commits the target once the update steps succeeded, and otherwise puts
// the installed release's manifest back on the row.
// quiver.core's own update is excluded: its relaunched binary adopts its new
// state on boot.
//
// The settling runs detached because this handler is delivered on the
// runtime aggregate's own ordered event queue, and clearing the badge waits
// for that same queue: done inline, it would wait for itself. Detached is not
// untracked: the row reads settling until it is done, and a shutdown drains
// it before closing the stores.
func (s *settler) OnUpdateEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	admitted := s.commits.Begin(rt.Ref)
	target, recorded := s.targets.Take(rt.Ref)
	if !admitted {
		slog.WarnContext(ctx, "update: shutting down; the row stays outdated until its next update", "ns", rt.Ref)
		return
	}

	s.detach(func() {
		settleCtx, cancel := s.commits.Bound(context.WithoutCancel(ctx), s.commitTimeout)
		defer cancel()
		defer s.releaseSettled(settleCtx, rt.Ref)
		s.settleUpdate(settleCtx, rt, target, recorded)
	})
}

// releaseSettled re-derives ns's badge from the row and lets the row go. A
// check held before a reconcile is covered by it, since the reconcile reads
// the row afterwards; only a check held while the reconcile ran may have
// recorded something it did not read, and only then is it run again. Once
// the row is released, checks sync the badge themselves.
func (s *settler) releaseSettled(
	ctx context.Context,
	ns domain.Namespace,
) {
	for {
		s.holds.mu.Lock()
		delete(s.holds.dirty, ns)
		s.holds.mu.Unlock()

		s.reconcileBadge(ctx, ns)

		s.holds.mu.Lock()
		if !s.holds.dirty[ns] {
			s.commits.Done(ns)
			s.holds.mu.Unlock()
			return
		}
		s.holds.mu.Unlock()
	}
}

// settleUpdate commits a succeeded update, or restores the installed
// release's manifest when nothing is stamped: a failed update, or one whose
// target moved, leaves the row reporting what it had installed, so the next
// install or execution runs that release's own steps for its own ${REF}.
func (s *settler) settleUpdate(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
	target domain.Available,
	recorded bool,
) {
	succeeded := rt.LastReturn != nil && rt.LastReturn.Outcome == domainRuntime.ExecutionOutcomeSuccess
	if isSelfNamespace(rt.Ref) {
		if !succeeded {
			s.restoreInstalled(ctx, rt.Ref)
		}
		return
	}
	if !recorded {
		if succeeded {
			slog.WarnContext(ctx, "update: no target recorded for a finished update", "ns", rt.Ref)
		}
		return
	}
	if succeeded && s.commitUpdate(ctx, rt.Ref, target) {
		return
	}
	restoreCtx, cancel, ok := s.afterSettle(ctx)
	if !ok {
		return
	}
	defer cancel()
	s.restoreInstalled(restoreCtx, rt.Ref)
}

// afterSettle gives the writes that close a settling their own context once
// the settling's ran out: a commit that timed out still restores the row and
// reconciles its badge. That context is bounded like a commit's, so a drain
// that gives up aborts it too, and a settling a shutdown drain aborted writes
// nothing: the stores are about to close.
func (s *settler) afterSettle(
	ctx context.Context,
) (context.Context, context.CancelFunc, bool) {
	if ctx.Err() == nil {
		return ctx, func() {}, true
	}
	if s.commits.IsDraining() {
		return nil, nil, false
	}
	fresh, cancel := s.commits.Bound(context.WithoutCancel(ctx), restoreTimeout)
	return fresh, cancel, true
}

// commitUpdate stamps target as installed only if it is still what its ref
// names: when the target moved while its update ran, the installed bits are
// not the target's, so nothing is stamped and the row stays outdated. The
// worst case is an extra update, never a missed one. It reports whether the
// row advanced.
func (s *settler) commitUpdate(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) bool {
	unmoved, err := s.arrow.TargetUnmoved(ctx, ns, target)
	if err != nil {
		slog.WarnContext(ctx, "update: re-resolve target", "ns", ns, "err", err)
		return false
	}
	if !unmoved {
		slog.WarnContext(ctx, "update: target moved during update",
			"ns", ns, "ref", target.Ref, "commit", target.Commit)
		return false
	}

	if err := s.arrow.Advance(ctx, ns, target); err != nil {
		slog.ErrorContext(ctx, "update: advance", "ns", ns, "err", err)
		return false
	}
	return true
}

// reconcileBadge lands the runtime badge on the row as the settled update
// left it: Ready once nothing is available, Outdated while the row still
// has something ahead (a newer release recorded during the update, or a
// target that was not stamped). See afterSettle for a settling that ran out.
func (s *settler) reconcileBadge(
	ctx context.Context,
	ns domain.Namespace,
) {
	ctx, cancel, ok := s.afterSettle(ctx)
	if !ok {
		return
	}
	defer cancel()
	if err := s.runtime.ReconcileVersionBadge(ctx, ns); err != nil {
		slog.ErrorContext(ctx, "update: reconcile version badge", "ns", ns, "err", err)
	}
}

// restoreInstalled stages the manifest of the release the row has installed
// again, replacing the target manifest the bracket staged. A row that
// recorded no commit (one written before commits were recorded) has nothing
// to fetch it at and keeps the staged manifest until its next update.
func (s *settler) restoreInstalled(
	ctx context.Context,
	ns domain.Namespace,
) {
	row, err := s.arrow.Get(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "update: read row to restore its installed manifest", "ns", ns, "err", err)
		return
	}
	if row == nil || row.Resolved.Commit == "" {
		return
	}
	installed := domain.Available{Ref: row.Resolved.Ref, Commit: row.Resolved.Commit}
	if _, err := s.arrow.RefreshToTarget(ctx, ns, installed); err != nil {
		slog.WarnContext(ctx, "update: restore the installed manifest", "ns", ns, "err", err)
	}
}

// isSelfNamespace reports whether ns is a ref of quiver.core's own self-arrow
// namespace; the "@" matters, or any namespace merely starting with it would match.
func isSelfNamespace(ns domain.Namespace) bool {
	self, _ := metadata.GetSelfNamespaces()
	return strings.HasPrefix(ns.String(), string(self)+"@")
}
