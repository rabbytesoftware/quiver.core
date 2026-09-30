package settle

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

const rollingRow = domain.Namespace("github.com/char2cs/crowbar@nightly-latest")

func rollingTarget() domain.Available {
	return domain.Available{Ref: "nightly-latest", Commit: "c2"}
}

// callLog records the order the bracket's steps run in.
type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(call string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, call)
}

func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

// bracketFixture wires the mocks an update bracket touches, logging each step.
type bracketFixture struct {
	log       *callLog
	arrow     *mocks.MockArrow
	runtime   *mocks.MockRuntime
	graph     *mocks.MockGraph
	state     domain.ArrowState
	available *domain.Available
	stateMu   sync.Mutex
}

func newBracketFixture(state domain.ArrowState, available *domain.Available) *bracketFixture {
	f := &bracketFixture{log: &callLog{}, state: state, available: available}
	f.arrow = &mocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns}, nil
		},
		CheckAvailableFn: func(context.Context, domain.Namespace) (*domain.Available, error) {
			f.log.add("check available")
			return f.available, nil
		},
		RefreshToTargetFn: func(_ context.Context, ns domain.Namespace, target domain.Available) (*domain.Arrow, error) {
			f.log.add("refresh to " + target.Commit)
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	f.runtime = &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return f.currentState(), nil
		},
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{Ref: ns, State: f.currentState()}, nil
		},
		ListenEndedFn: func(context.Context, domain.Namespace) (<-chan domainRuntime.ArrowRuntime, func(), error) {
			ch := make(chan domainRuntime.ArrowRuntime, 1)
			ch <- domainRuntime.ArrowRuntime{}
			return ch, func() {}, nil
		},
		BeginStopFn: func(context.Context, domain.Namespace) error {
			f.log.add("stop")
			f.setState(domain.ArrowStateReady)
			return nil
		},
		BeginUpdateFn: func(context.Context, domain.Namespace, map[string]string, string) error {
			f.log.add("begin update")
			return nil
		},
	}
	f.graph = &mocks.MockGraph{}
	return f
}

func (f *bracketFixture) currentState() domain.ArrowState {
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	return f.state
}

func (f *bracketFixture) setState(state domain.ArrowState) {
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	f.state = state
}

func (f *bracketFixture) usecase() *harness {
	return newUC(f.arrow, f.runtime, f.graph)
}

func updateEnded(ns domain.Namespace, outcome domainRuntime.ExecutionOutcome) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        ns,
		LastReturn: &domainRuntime.Return{Method: domain.MethodUpdate, Outcome: outcome},
	}
}

// commitFixture logs every step onUpdateEnded may take. The row has c1
// installed.
func commitFixture(unmoved bool, unmovedErr error) (*mocks.MockArrow, *mocks.MockRuntime, *callLog) {
	log := &callLog{}
	a := &mocks.MockArrow{
		GetFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
		},
		TargetUnmovedFn: func(_ context.Context, _ domain.Namespace, target domain.Available) (bool, error) {
			log.add("re-resolve " + target.Commit)
			return unmoved, unmovedErr
		},
		AdvanceFn: func(_ context.Context, _ domain.Namespace, target domain.Available) error {
			log.add("advance " + target.Commit)
			return nil
		},
		RefreshToTargetFn: func(_ context.Context, ns domain.Namespace, target domain.Available) (*domain.Arrow, error) {
			log.add("restore " + target.Commit)
			return &domain.Arrow{Namespace: ns}, nil
		},
	}
	rt := &mocks.MockRuntime{
		ReconcileVersionBadgeFn: func(context.Context, domain.Namespace) error {
			log.add("reconcile badge")
			return nil
		},
	}
	return a, rt, log
}
