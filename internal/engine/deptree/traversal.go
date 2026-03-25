package deptree

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type traversal struct {
	ctx      context.Context
	resolver ResolverFunc
	state    map[domain.Namespace]nodeState
	stack    []domain.Namespace
	order    []domain.Namespace
}

func NewTraversal(
	ctx context.Context,
	resolver ResolverFunc,
) traversal {
	return traversal{
		ctx:      ctx,
		resolver: resolver,
		state:    make(map[domain.Namespace]nodeState, 16),
		stack:    make([]domain.Namespace, 0, 16),
		order:    make([]domain.Namespace, 0, 16),
	}
}

func (t *traversal) dfs(
	ns domain.Namespace,
) error {
	if t.state[ns] == done {
		return nil
	}

	if t.state[ns] == inProgress {
		return t.buildCycleError(ns)
	}

	if err := t.ctx.Err(); err != nil {
		return err
	}

	t.state[ns] = inProgress
	t.stack = append(t.stack, ns)

	deps, err := t.resolver(t.ctx, ns)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		if err := t.dfs(dep); err != nil {
			return err
		}
	}

	t.stack = t.stack[:len(t.stack)-1]
	t.state[ns] = done
	t.order = append(t.order, ns)

	return nil
}

func (t *traversal) buildCycleError(
	ns domain.Namespace,
) *CycleError {
	cycleStart := 0
	for i, n := range t.stack {
		if n == ns {
			cycleStart = i
			break
		}
	}

	path := make([]domain.Namespace, len(t.stack)-cycleStart+1)
	copy(path, t.stack[cycleStart:])
	path[len(path)-1] = ns

	return &CycleError{Path: path}
}
