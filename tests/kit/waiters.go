//go:build integration

// Choosing a timeout for anything in this package or in Env's waiters: the
// argument is a failure deadline, not a delay. Every waiter returns the instant
// its condition holds, so a generous bound costs nothing on a passing run and
// only decides how long a genuine hang takes to report.
//
// Use 120s. The suite previously mixed 5s, 10s, 15s and 30s with no rationale,
// tuned on developer hardware; a two-core CI runner under -race is roughly an
// order of magnitude slower, and TestDeps_ExportsInjectedToConsumer duly timed
// out at 30s there while passing in four seconds locally.
//
// A shorter bound is only correct when the shortness *is* the assertion — the
// single such case is oracle's 1s, which exists to prove the read model is
// current, and it says so at the call site.
package kit

import (
	"fmt"
	"testing"
	"time"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// pollTick is the retry cadence of the REST-backed waiters in this file. It is a
// retry interval, never a settling delay: every waiter evaluates its condition
// before the first tick and returns the instant the condition holds, so the
// cadence only bounds how long a waiter lingers after the state it wants landed.
const pollTick = 5 * time.Millisecond

// WaitForDetail re-reads GET /v0/arrow/:ns until cond accepts the response and
// returns that exact snapshot, so callers assert on the state they waited for
// rather than on a later re-read that may already have moved on.
//
// Polling — not the WebSocket watchers in env.go — is the honest primitive for
// the arrow read model. installed_at and the list rows are written by asynx
// projections that run independently of the hub broadcast, so no stream event
// proves the REST response has caught up. The wait is still bounded by a
// condition rather than by a duration; `want` only names the condition for the
// timeout message.
func WaitForDetail(
	t *testing.T,
	tc *TypedClient,
	ns string,
	want string,
	timeout time.Duration,
	cond func(detail dto.ArrowDetailDTO, status int) bool,
) dto.ArrowDetailDTO {
	t.Helper()

	ticker := time.NewTicker(pollTick)
	defer ticker.Stop()
	expired := time.After(timeout)

	for {
		detail, status := tc.GetDetail(ns)
		if cond(detail, status) {
			return detail
		}
		select {
		case <-ticker.C:
		case <-expired:
			t.Fatalf(
				"WaitForDetail(%s): timeout after %s waiting for %s (last: status=%d state=%q)",
				ns, timeout, want, status, detail.State,
			)
			return detail
		}
	}
}

// WaitForList re-reads GET /v0/arrow until cond accepts the catalog and returns
// that exact snapshot. See WaitForDetail for why this polls.
func WaitForList(
	t *testing.T,
	tc *TypedClient,
	want string,
	timeout time.Duration,
	cond func(items []dto.ArrowListItemDTO, status int) bool,
) []dto.ArrowListItemDTO {
	t.Helper()

	ticker := time.NewTicker(pollTick)
	defer ticker.Stop()
	expired := time.After(timeout)

	for {
		items, status := tc.List()
		if cond(items, status) {
			return items
		}
		select {
		case <-ticker.C:
		case <-expired:
			t.Fatalf(
				"WaitForList: timeout after %s waiting for %s (last: status=%d items=%d)",
				timeout, want, status, len(items),
			)
			return items
		}
	}
}

// WaitForLastReturn waits until the arrow reports a last_return holding at least
// minSteps steps. minSteps == 0 waits only for last_return to exist. The runtime
// writes last_return when an execution ends, one command after the transition
// the WebSocket state watcher observes, so reaching `ready` does not imply it.
func WaitForLastReturn(
	t *testing.T,
	tc *TypedClient,
	ns string,
	minSteps int,
	timeout time.Duration,
) dto.ArrowDetailDTO {
	t.Helper()

	return WaitForDetail(
		t, tc, ns,
		fmt.Sprintf("last_return with >= %d steps", minSteps),
		timeout,
		func(detail dto.ArrowDetailDTO, _ int) bool {
			return detail.LastReturn != nil && len(detail.LastReturn.Steps) >= minSteps
		},
	)
}
