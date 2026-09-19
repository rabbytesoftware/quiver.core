//go:build integration

package kit

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// pidRegistry tracks the most recent PID recorded for each namespace, fed
// exclusively by pidCapturingWizard. It only ever holds PIDs for
// non-one-shot executions (_execute and any custom manifest method) — see
// pidCapturingWizard.Start — so a caller never needs to re-filter it before
// deciding what is safe to kill.
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
// process's PID synchronously, at the moment the wizard's own steprun
// handler emits EventKindPID — entirely independent of the app layer's
// RecordPID command, which can be silently dropped
// (apperrors.ErrExecutionSuperseded, see
// internal/app/repositories/runtime/internal/hooks.go's sendPID) when a
// _stop's PID snapshot races an in-flight execution's own RecordPID. That
// drop is not a timing fluke to out-wait: once the command is dropped, the
// app layer's read model never learns the PID, no matter how long anything
// downstream waits. Observing the wizard's own event stream directly has no
// such failure mode — the event either hasn't happened yet (wait for it) or
// it has (and this always sees it).
//
// Only non-one-shot methods are wrapped and recorded: those are the only
// ones wizard.Shutdown itself does not already cancel (see
// wizard.IsOneShotMethod), so they are the only ones a test's Close() is
// responsible for cleaning up itself. Wrapping and recording an
// _install/_uninstall/_update/_stop PID here would make killSurvivingProcesses
// hard-kill a process wizard.Shutdown was already gracefully cancelling on
// its own — exactly the regression this scoping avoids.
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

// killSurvivingProcesses force-kills any OS process pids still knows about
// that is still alive. wizard.Shutdown only cancels the four one-shot
// lifecycle methods (_install/_uninstall/_update/_stop): an _execute or
// custom-method execution is a supervised process meant to outlive a
// production daemon shutdown, so nothing in Close's graceful sequence ever
// asks it to exit. A finished test still needs the fixture-spawned process
// gone — unlike a real self-update relaunch, the test binary never re-execs
// to reclaim it. pids only ever holds non-one-shot PIDs (see
// pidCapturingWizard), so this never touches a process wizard.Shutdown was
// already going to cancel gracefully on its own.
//
// BeginExecution flips a namespace's state to Running before the wizard has
// even spawned its process and emitted a PID — so WaitForState(Running)
// alone does not guarantee pids already has the PID for a namespace still
// Running at Close time. waitForPendingPIDs closes that gap for exactly
// those namespaces, bounded, rather than either racing ahead immediately or
// padding every test's teardown with a blind sleep.
func killSurvivingProcesses(states *stateWatcher, pids *pidRegistry, wiz wizard.Wizard) {
	waitForPendingPIDs(states, pids, wiz, pidWaitTimeout)

	for _, pid := range pids.snapshot() {
		if !wiz.ProcessAlive(pid) {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
		}
	}
}

// waitForPendingPIDs blocks, up to timeout, until every namespace states
// last reported as Running also has an alive PID recorded in pids. Presence
// alone is not enough to check: pids is keyed by namespace, so a namespace
// executed more than once within a single test (e.g. Execute, Stop, Execute
// again) still has an entry the whole time — the stale PID from the earlier
// run — until the newer PID event overwrites it. Checking liveness, not
// just presence, is what actually waits for that overwrite. A namespace
// that never gets a live PID within the budget is simply skipped by the
// caller's subsequent snapshot.
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
