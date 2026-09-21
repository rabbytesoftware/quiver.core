//go:build integration

package kit

import (
	"context"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// pidRegistry tracks the most recent PID recorded for each namespace, fed
// exclusively by pidCapturingWizard, and only ever holds non-one-shot PIDs.
type pidRegistry struct {
	mu   sync.Mutex
	pids map[string]int
}

func newPIDRegistry() *pidRegistry {
	return &pidRegistry{pids: make(map[string]int)}
}

func (r *pidRegistry) record(ns string, pid int) {
	if pid <= 0 {
		return
	}
	r.mu.Lock()
	r.pids[ns] = pid
	r.mu.Unlock()
}

// get returns the most recently recorded PID for ns, if any.
func (r *pidRegistry) get(ns string) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pid, ok := r.pids[ns]
	return pid, ok
}

// snapshot returns every PID this registry currently holds. A returned PID
// may already have exited — callers must confirm liveness before acting on it.
func (r *pidRegistry) snapshot() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, 0, len(r.pids))
	for _, pid := range r.pids {
		out = append(out, pid)
	}
	return out
}

// pidCapturingWizard wraps a real wizard.Wizard to observe every spawned
// process's PID synchronously off its own event stream, since the app
// layer's RecordPID command can be silently dropped by a racing _stop (see
// hooks.go's sendPID). Only non-one-shot methods are recorded, since those
// are the only ones wizard.Shutdown doesn't already cancel on its own.
type pidCapturingWizard struct {
	wizard.Wizard
	pids *pidRegistry
}

// newPIDCapturingWizard wraps w so BuildEnv can pass the result to app.New
// in place of the real engine-built wizard, before any execution starts.
func newPIDCapturingWizard(w wizard.Wizard, pids *pidRegistry) wizard.Wizard {
	return &pidCapturingWizard{Wizard: w, pids: pids}
}

func (w *pidCapturingWizard) Start(ctx context.Context, req wizard.RunRequest) wizard.Execution {
	exec := w.Wizard.Start(ctx, req)
	if wizard.IsOneShotMethod(req.Method) {
		return exec
	}
	return &pidSniffingExecution{
		Execution: exec,
		ns:        req.Namespace.String(),
		pids:      w.pids,
	}
}

// pidSniffingExecution proxies Execution.Events() through a forwarding
// goroutine that records EventKindPID synchronously before passing every
// event on untouched, so the real consumer (the runtime repository's
// drainExecution) sees exactly the stream it always did — this is purely an
// observer, never a filter.
type pidSniffingExecution struct {
	wizard.Execution
	ns   string
	pids *pidRegistry

	once sync.Once
	teed chan wizard.Event
}

func (e *pidSniffingExecution) Events() <-chan wizard.Event {
	e.once.Do(func() {
		e.teed = make(chan wizard.Event, 16)
		go func() {
			defer close(e.teed)
			for ev := range e.Execution.Events() {
				if ev.Kind == wizard.EventKindPID {
					e.pids.record(e.ns, ev.PID)
				}
				e.teed <- ev
			}
		}()
	})
	return e.teed
}

// pidWaitTimeout bounds how long killSurvivingProcesses waits for a Running
// namespace's PID to land in pids before giving up on it.
const pidWaitTimeout = 2 * time.Second

// killSurvivingProcesses force-kills any still-alive OS process pids knows
// about: an _execute or custom-method process is meant to outlive a
// production shutdown, so wizard.Shutdown never asks it to exit, but a
// finished test still needs it gone.
func killSurvivingProcesses(states *stateWatcher, pids *pidRegistry, wiz wizard.Wizard) {
	waitForPendingPIDs(states, pids, wiz, pidWaitTimeout)

	for _, pid := range pids.snapshot() {
		if !wiz.ProcessAlive(pid) {
			continue
		}
		killProcessGroup(pid)
	}
}

// waitForPendingPIDs blocks, up to timeout, until every Running namespace
// has an alive PID recorded — checking liveness, not just presence, since a
// re-executed namespace keeps a stale PID entry until the newer one lands.
func waitForPendingPIDs(states *stateWatcher, pids *pidRegistry, wiz wizard.Wizard, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		pending := false
		for _, ns := range states.namespacesInState(domain.ArrowStateRunning) {
			pid, ok := pids.get(ns)
			if !ok || !wiz.ProcessAlive(pid) {
				pending = true
				break
			}
		}
		if !pending || time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
