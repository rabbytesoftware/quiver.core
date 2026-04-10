package deptree

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ns(
	label string,
) domain.Namespace {
	return domain.Namespace(fmt.Sprintf("github.com/owner/%s", label))
}

func buildResolver(
	graph map[domain.Namespace][]domain.Namespace,
) ResolverFunc {
	return func(_ context.Context, n domain.Namespace) ([]domain.Namespace, error) {
		deps, ok := graph[n]
		if !ok {
			return []domain.Namespace{}, nil
		}
		return deps, nil
	}
}

func TestDeptree(t *testing.T) {
	result, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(nil),
	)

	require.NoError(t, err)
	require.Equal(t, []domain.Namespace{ns("a")}, result)
}

func TestDeptree_LinearChain(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b")},
		ns("b"): {ns("c")},
		ns("c"): {ns("d")},
	}

	result, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(graph),
	)

	require.NoError(t, err)
	require.Equal(t, []domain.Namespace{ns("d"), ns("c"), ns("b"), ns("a")}, result)
}

func TestDeptree_DiamondDependency(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b"), ns("c")},
		ns("b"): {ns("d")},
		ns("c"): {ns("d")},
	}

	result, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(graph),
	)

	require.NoError(t, err)
	require.Equal(t, ns("a"), result[len(result)-1])

	count := 0
	for _, n := range result {
		if n == ns("d") {
			count++
		}
	}
	assert.Equal(t, 1, count)
	assert.Len(t, result, 4)
}

func TestDeptree_WideDependencies(t *testing.T) {
	deps := make([]domain.Namespace, 10)
	for i := range deps {
		deps[i] = domain.Namespace(fmt.Sprintf("github.com/owner/dep%d", i))
	}
	graph := map[domain.Namespace][]domain.Namespace{
		ns("root"): deps,
	}

	result, err := New().Resolve(
		context.Background(),
		ns("root"),
		buildResolver(graph),
	)

	require.NoError(t, err)
	assert.Len(t, result, 11)
	assert.Equal(t, ns("root"), result[len(result)-1])
}

func TestDeptree_DeepTransitive(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b")},
		ns("b"): {ns("c")},
		ns("c"): {ns("d")},
		ns("d"): {ns("e")},
	}

	result, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(graph),
	)

	require.NoError(t, err)
	require.Len(t, result, 5)
	assert.Equal(t, []domain.Namespace{ns("e"), ns("d"), ns("c"), ns("b"), ns("a")}, result)
}

func TestDeptree_CyclicDependency(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b")},
		ns("b"): {ns("c")},
		ns("c"): {ns("a")},
	}

	_, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(graph),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCyclicDependency)

	var cycleErr *CycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, ns("a"), cycleErr.Path[0])
	assert.Equal(t, ns("a"), cycleErr.Path[len(cycleErr.Path)-1])
	assert.Contains(t, err.Error(), "cyclic dependency detected")
}

func TestDeptree_SelfDependency(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("a")},
	}

	_, err := New().Resolve(
		context.Background(),
		ns("a"),
		buildResolver(graph),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCyclicDependency)

	var cycleErr *CycleError
	require.ErrorAs(t, err, &cycleErr)
	assert.Equal(t, []domain.Namespace{ns("a"), ns("a")}, cycleErr.Path)
}

func TestDeptree_ResolverError(t *testing.T) {
	sentinel := errors.New("manifest fetch failed")
	resolver := func(_ context.Context, n domain.Namespace) ([]domain.Namespace, error) {
		if n == ns("b") {
			return nil, sentinel
		}
		return []domain.Namespace{ns("b")}, nil
	}

	_, err := New().Resolve(
		context.Background(),
		ns("a"),
		resolver,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestDeptree_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b")},
	}

	_, err := New().Resolve(
		ctx,
		ns("a"),
		buildResolver(graph),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDeptree_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b")},
	}

	_, err := New().Resolve(
		ctx,
		ns("a"),
		buildResolver(graph),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestDeptree_DeterministicOrder(t *testing.T) {
	graph := map[domain.Namespace][]domain.Namespace{
		ns("a"): {ns("b"), ns("c")},
		ns("b"): {ns("d")},
		ns("c"): {ns("d")},
	}
	resolver := buildResolver(graph)

	dt := New()
	result1, err1 := dt.Resolve(context.Background(), ns("a"), resolver)
	result2, err2 := dt.Resolve(context.Background(), ns("a"), resolver)

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, result1, result2)
}

func TestDeptree_RootAlwaysLast(t *testing.T) {
	tests := []struct {
		name  string
		root  domain.Namespace
		graph map[domain.Namespace][]domain.Namespace
	}{
		{
			name:  "no deps",
			root:  ns("a"),
			graph: nil,
		},
		{
			name:  "linear chain",
			root:  ns("a"),
			graph: map[domain.Namespace][]domain.Namespace{ns("a"): {ns("b")}, ns("b"): {ns("c")}},
		},
		{
			name: "diamond",
			root: ns("a"),
			graph: map[domain.Namespace][]domain.Namespace{
				ns("a"): {ns("b"), ns("c")},
				ns("b"): {ns("d")},
				ns("c"): {ns("d")},
			},
		},
	}

	dt := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := dt.Resolve(
				context.Background(),
				tt.root,
				buildResolver(tt.graph),
			)

			require.NoError(t, err)
			require.NotEmpty(t, result)
			assert.Equal(t, tt.root, result[len(result)-1])
		})
	}
}

func TestCycleError_Error(t *testing.T) {
	err := &CycleError{Path: []domain.Namespace{ns("a"), ns("b"), ns("a")}}

	msg := err.Error()

	assert.Contains(t, msg, "cyclic dependency detected")
	assert.Contains(t, msg, "github.com/owner/a")
	assert.Contains(t, msg, "->")
}

func TestCycleError_Unwrap(t *testing.T) {
	err := &CycleError{Path: []domain.Namespace{ns("a"), ns("a")}}

	assert.ErrorIs(t, err, ErrCyclicDependency)
}
