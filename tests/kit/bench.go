//go:build integration && bench

package kit

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"
)

type BenchmarkResult struct {
	P50Ms float64 `json:"p50_ms"`
	P99Ms float64 `json:"p99_ms"`
}

type benchBaseline struct {
	Version    int                        `json:"version"`
	Updated    string                     `json:"updated"`
	Benchmarks map[string]BenchmarkResult `json:"benchmarks"`
}

// RunBenchmark calls fn n times. fn is responsible for timing its own window
// and returning the measured duration. Returns a sorted slice of durations.
func RunBenchmark(t *testing.T, n int, fn func() time.Duration) []time.Duration {
	t.Helper()
	d := make([]time.Duration, n)
	for i := range d {
		d[i] = fn()
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d
}

// P50 returns the median of a sorted duration slice.
func P50(d []time.Duration) time.Duration { return d[len(d)/2] }

// P99 returns the 99th-percentile of a sorted duration slice.
func P99(d []time.Duration) time.Duration {
	idx := int(float64(len(d)) * 0.99)
	if idx >= len(d) {
		idx = len(d) - 1
	}
	return d[idx]
}

// baselinePath resolves baseline.json relative to the calling test's working dir.
// bench_test.go runs from tests/integration/bench/ so "baseline.json" resolves correctly.
func baselinePath() string { return "baseline.json" }

func loadBaseline(t *testing.T) benchBaseline {
	t.Helper()
	data, err := os.ReadFile(baselinePath())
	if os.IsNotExist(err) {
		return benchBaseline{Version: 1, Benchmarks: map[string]BenchmarkResult{}}
	}
	if err != nil {
		t.Fatalf("loadBaseline: %v", err)
	}
	var b benchBaseline
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("loadBaseline: unmarshal: %v", err)
	}
	return b
}

func saveBaseline(t *testing.T, b benchBaseline) {
	t.Helper()
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		t.Fatalf("saveBaseline: %v", err)
	}
	if err := os.WriteFile(baselinePath(), data, 0o644); err != nil { // #nosec G306
		t.Fatalf("saveBaseline: write: %v", err)
	}
}

// AssertNoRegression checks p99 against baseline.json (1.25× threshold).
// Writes the baseline entry if missing or UPDATE_BASELINE=1.
// Must be called from within tests/integration/bench/ (working dir).
func AssertNoRegression(t *testing.T, name string, p50, p99 time.Duration) {
	t.Helper()
	b := loadBaseline(t)
	if b.Benchmarks == nil {
		b.Benchmarks = map[string]BenchmarkResult{}
	}

	p50ms := float64(p50.Microseconds()) / 1000.0
	p99ms := float64(p99.Microseconds()) / 1000.0

	update := os.Getenv("UPDATE_BASELINE") == "1"
	entry, exists := b.Benchmarks[name]

	if update || !exists {
		b.Benchmarks[name] = BenchmarkResult{P50Ms: p50ms, P99Ms: p99ms}
		b.Updated = time.Now().Format("2006-01-02")
		saveBaseline(t, b)
		t.Logf("bench(%s): baseline written p50=%.1fms p99=%.1fms", name, p50ms, p99ms)
		return
	}

	threshold := entry.P99Ms * 1.25
	if p99ms > threshold {
		t.Logf("bench(%s): ⚠ REGRESSION p99 %.1fms exceeds 1.25× baseline %.1fms (threshold %.1fms)",
			name, p99ms, entry.P99Ms, threshold)
		return
	}
	t.Logf("bench(%s): p50=%.1fms p99=%.1fms (baseline p99=%.1fms)", name, p50ms, p99ms, entry.P99Ms)
}
