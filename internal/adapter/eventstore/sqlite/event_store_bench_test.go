package store

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkEventStore_Append(
	b *testing.B,
) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := NewEventStore(":memory:")
		if err != nil {
			b.Fatal(err)
		}
		for v := int64(1); v <= 1000; v++ {
			_ = s.Append(ctx, "agg-1", v, []byte("event-data"))
		}
	}
}

func BenchmarkEventStore_ReadFrom(
	b *testing.B,
) {
	ctx := context.Background()
	s, err := NewEventStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	for v := int64(1); v <= 1000; v++ {
		_ = s.Append(ctx, "agg-1", v, []byte(fmt.Sprintf("event-%d", v)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.ReadFrom(ctx, "agg-1", 1)
	}
}
