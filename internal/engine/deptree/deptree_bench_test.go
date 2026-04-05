package deptree

import (
	"context"
	"fmt"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func BenchmarkDeptree_LinearChain10(b *testing.B) {
	nodes := make([]domain.Namespace, 10)
	for i := range nodes {
		nodes[i] = domain.Namespace(fmt.Sprintf("github.com/owner/node%d", i))
	}
	graph := make(map[domain.Namespace][]domain.Namespace)
	for i := 0; i < len(nodes)-1; i++ {
		graph[nodes[i]] = []domain.Namespace{nodes[i+1]}
	}
	resolver := buildResolver(graph)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = New().Resolve(
			ctx,
			nodes[0],
			resolver,
		)
	}
}

func BenchmarkDeptree_DiamondDependency(b *testing.B) {
	a := domain.Namespace("github.com/owner/a")
	bNode := domain.Namespace("github.com/owner/b")
	c := domain.Namespace("github.com/owner/c")
	d := domain.Namespace("github.com/owner/d")
	graph := map[domain.Namespace][]domain.Namespace{
		a:     {bNode, c},
		bNode: {d},
		c:     {d},
	}
	resolver := buildResolver(graph)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = New().Resolve(
			ctx,
			a,
			resolver,
		)
	}
}

func BenchmarkDeptree_Wide50Deps(b *testing.B) {
	root := domain.Namespace("github.com/owner/root")
	deps := make([]domain.Namespace, 50)
	for i := range deps {
		deps[i] = domain.Namespace(fmt.Sprintf("github.com/owner/dep%d", i))
	}
	graph := map[domain.Namespace][]domain.Namespace{
		root: deps,
	}
	resolver := buildResolver(graph)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = New().Resolve(
			ctx,
			root,
			resolver,
		)
	}
}
