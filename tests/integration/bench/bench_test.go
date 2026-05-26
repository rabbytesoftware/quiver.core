//go:build integration && bench

package bench_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

var (
	reposOnce sync.Once
	repos     *kit.FixtureRepos
)

func getRepos(t *testing.T) *kit.FixtureRepos {
	t.Helper()
	reposOnce.Do(func() { repos = kit.BuildFixtureRepos(t) })
	return repos
}

func newEnv(t *testing.T) (*kit.Env, *kit.TypedClient) {
	t.Helper()
	env := kit.BuildEnv(t, getRepos(t), nil, t.TempDir())
	return env, env.TypedClient(t)
}

// arrowYAML builds a minimal install-only arrow manifest.
// deps are fully-qualified namespaces listed under tools.
func arrowYAML(name string, deps ...string) []byte {
	tools := ""
	for _, d := range deps {
		tools += fmt.Sprintf("      - %s\n", d)
	}
	toolsSection := ""
	if tools != "" {
		toolsSection = "    tools:\n" + tools
	}
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: %s
  version: 1.0.0
  description: bench arrow
targets:
  "*":
%s    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, name, toolsSection))
}

// seedAndWait seeds an arrow and blocks until the catalog WS event arrives.
func seedAndWait(t *testing.T, env *kit.Env, tc *kit.TypedClient, ns, name string, deps ...string) {
	t.Helper()
	tc.Seed(ns, arrowYAML(name, deps...))
	env.WaitForArrow(t, ns, 30*time.Second)
}

// installAndWait installs an arrow and blocks until the WS ready event arrives.
func installAndWait(t *testing.T, env *kit.Env, tc *kit.TypedClient, ns string) {
	t.Helper()
	tc.Install(ns, nil)
	env.WaitForState(t, ns, domain.ArrowStateReady, 60*time.Second)
}

// uninstallAndWait uninstalls an arrow and blocks until the WS absent event arrives.
func uninstallAndWait(t *testing.T, env *kit.Env, tc *kit.TypedClient, ns string) {
	t.Helper()
	tc.Uninstall(ns, nil)
	env.WaitForState(t, ns, domain.ArrowStateAbsent, 30*time.Second)
}

// ─── Add ─────────────────────────────────────────────────────────────────────

// TestBench_Add measures POST /v0/arrow/:ns/seed → catalog WS event.
// Each iteration uses a unique namespace so each run exercises a fresh asynx write.
func TestBench_Add(t *testing.T) {
	env, tc := newEnv(t)
	counter := 0

	durations := kit.RunBenchmark(t, 50, func() time.Duration {
		counter++
		ns := fmt.Sprintf("quiver.test/bench/add-%d@v1", counter)
		name := fmt.Sprintf("bench.add-%d", counter)

		start := time.Now()
		tc.Seed(ns, arrowYAML(name))
		env.WaitForArrow(t, ns, 30*time.Second)
		return time.Since(start)
	})

	kit.AssertNoRegression(t, "Add", kit.P50(durations), kit.P99(durations))
}

// ─── Install cold ─────────────────────────────────────────────────────────────

// TestBench_InstallCold measures a fresh install request → ready WS event.
// The arrow is seeded before each measured window; each run uses a unique namespace
// so no prior events exist in the asynx store (cold path).
func TestBench_InstallCold(t *testing.T) {
	env, tc := newEnv(t)
	counter := 0

	durations := kit.RunBenchmark(t, 20, func() time.Duration {
		counter++
		ns := fmt.Sprintf("quiver.test/bench/cold-%d@v1", counter)
		name := fmt.Sprintf("bench.cold-%d", counter)

		seedAndWait(t, env, tc, ns, name)

		start := time.Now()
		tc.Install(ns, nil)
		env.WaitForState(t, ns, domain.ArrowStateReady, 30*time.Second)
		return time.Since(start)
	})

	kit.AssertNoRegression(t, "InstallCold", kit.P50(durations), kit.P99(durations))
}

// ─── Install warm ─────────────────────────────────────────────────────────────

// TestBench_InstallWarm measures a re-install on a namespace that already has
// asynx events and a snapshot from a prior install+uninstall cycle.
// The inter-iteration reset (uninstall → absent) is excluded from timing.
func TestBench_InstallWarm(t *testing.T) {
	env, tc := newEnv(t)
	ns := "quiver.test/bench/warm@v1"

	// Pre-warm: establish at least one snapshot in the asynx store.
	seedAndWait(t, env, tc, ns, "bench.warm")
	installAndWait(t, env, tc, ns)
	uninstallAndWait(t, env, tc, ns)

	durations := kit.RunBenchmark(t, 20, func() time.Duration {
		start := time.Now()
		tc.Install(ns, nil)
		env.WaitForState(t, ns, domain.ArrowStateReady, 30*time.Second)
		elapsed := time.Since(start)

		// Reset for next iteration (excluded from timing).
		uninstallAndWait(t, env, tc, ns)
		return elapsed
	})

	kit.AssertNoRegression(t, "InstallWarm", kit.P50(durations), kit.P99(durations))
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

// TestBench_GetDetail measures GET /v0/arrow/:ns/detail latency under varying
// concurrency. Uses net/http directly to avoid t.Fatal from goroutines.
func TestBench_GetDetail(t *testing.T) {
	env, tc := newEnv(t)
	ns := "quiver.test/bench/detail@v1"

	seedAndWait(t, env, tc, ns, "bench.detail")
	installAndWait(t, env, tc, ns)

	detailURL := env.URL + "/v0/arrow/" + url.PathEscape(ns) + "/detail"
	httpClient := env.HTTPClient(10 * time.Second)

	for _, concurrency := range []int{1, 10, 50} {
		n := concurrency
		name := fmt.Sprintf("GetDetail/concurrent-%d", n)

		durations := kit.RunBenchmark(t, 50, func() time.Duration {
			results := make([]time.Duration, n)
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					start := time.Now()
					resp, _ := httpClient.Get(detailURL) //nolint:noctx
					if resp != nil {
						resp.Body.Close()
					}
					results[i] = time.Since(start)
				}(i)
			}
			wg.Wait()
			sort.Slice(results, func(a, b int) bool { return results[a] < results[b] })
			return results[len(results)/2]
		})

		kit.AssertNoRegression(t, name, kit.P50(durations), kit.P99(durations))
	}
}

// ─── List ─────────────────────────────────────────────────────────────────────

// TestBench_List measures GET /v0/arrow latency with 10, 100, and 500 arrows
// in the catalog. Uses net/http directly to avoid t.Fatal from goroutines.
func TestBench_List(t *testing.T) {
	env, tc := newEnv(t)
	listURL := env.URL + "/v0/arrow"
	httpClient := env.HTTPClient(30 * time.Second)

	sizes := []int{10, 100, 500}
	seeded := 0

	for _, size := range sizes {
		// Seed arrows up to this size (cumulative).
		for seeded < size {
			seeded++
			ns := fmt.Sprintf("quiver.test/bench/list-%d@v1", seeded)
			name := fmt.Sprintf("bench.list-%d", seeded)
			tc.Seed(ns, arrowYAML(name))
		}
		env.WaitForCatalogLen(t, size, 60*time.Second)

		name := fmt.Sprintf("List/catalog-%d", size)
		durations := kit.RunBenchmark(t, 50, func() time.Duration {
			start := time.Now()
			resp, _ := httpClient.Get(listURL) //nolint:noctx
			if resp != nil {
				resp.Body.Close()
			}
			return time.Since(start)
		})

		kit.AssertNoRegression(t, name, kit.P50(durations), kit.P99(durations))
	}
}

// ─── Dep resolution ───────────────────────────────────────────────────────────

// TestBench_DepResolution measures install latency for arrows with varying
// dependency graph shapes. All deps are pre-installed so the timing captures
// dep resolution overhead (graph.Resolve DFS) and not dep install time.
func TestBench_DepResolution(t *testing.T) {
	env, tc := newEnv(t)

	// seed and pre-install a set of shared leaf deps
	seedInstallLeaf := func(ns, name string) {
		seedAndWait(t, env, tc, ns, name)
		installAndWait(t, env, tc, ns)
	}

	// ── linear chain: root → dep1 → dep2 → ... → depN ──────────────────────
	for _, depth := range []int{1, 5, 10} {
		// Seed leaf-to-root so dep edges are registered before root is seeded.
		var chain []string
		for i := depth; i >= 1; i-- {
			ns := fmt.Sprintf("quiver.test/bench/linear-d%d-dep%d@v1", depth, i)
			chain = append(chain, ns)
		}
		// Seed leaves first, each depending on the next.
		for idx, ns := range chain {
			name := fmt.Sprintf("bench.linear-d%d-dep%d", depth, depth-idx)
			var dep string
			if idx+1 < len(chain) {
				dep = chain[idx+1]
			}
			if dep != "" {
				seedAndWait(t, env, tc, ns, name, dep)
			} else {
				seedAndWait(t, env, tc, ns, name)
			}
		}
		// Pre-install all deps (leaves through second-to-root).
		for idx := len(chain) - 1; idx >= 1; idx-- {
			installAndWait(t, env, tc, chain[idx])
		}

		rootNS := chain[0]
		name := fmt.Sprintf("DepResolution/linear-depth-%d", depth)
		counter := 0
		durations := kit.RunBenchmark(t, 10, func() time.Duration {
			counter++
			// Use a unique root ns per run to avoid the idempotency guard returning early.
			// Only the root changes; shared deps remain pre-installed.
			rns := fmt.Sprintf("quiver.test/bench/linear-d%d-root-%d@v1", depth, counter)
			rname := fmt.Sprintf("bench.linear-d%d-root-%d", depth, counter)
			dep := rootNS // root depends on the pre-installed chain head
			seedAndWait(t, env, tc, rns, rname, dep)

			start := time.Now()
			tc.Install(rns, nil)
			env.WaitForState(t, rns, domain.ArrowStateReady, 30*time.Second)
			return time.Since(start)
		})

		kit.AssertNoRegression(t, name, kit.P50(durations), kit.P99(durations))
	}

	// ── wide graph: root depends on N leaves directly ────────────────────────
	for _, width := range []int{1, 5, 10} {
		var leaves []string
		for i := 1; i <= width; i++ {
			ns := fmt.Sprintf("quiver.test/bench/wide-w%d-leaf%d@v1", width, i)
			leaves = append(leaves, ns)
			seedInstallLeaf(ns, fmt.Sprintf("bench.wide-w%d-leaf%d", width, i))
		}

		name := fmt.Sprintf("DepResolution/wide-width-%d", width)
		counter := 0
		durations := kit.RunBenchmark(t, 10, func() time.Duration {
			counter++
			rns := fmt.Sprintf("quiver.test/bench/wide-w%d-root-%d@v1", width, counter)
			rname := fmt.Sprintf("bench.wide-w%d-root-%d", width, counter)
			seedAndWait(t, env, tc, rns, rname, leaves...)

			start := time.Now()
			tc.Install(rns, nil)
			env.WaitForState(t, rns, domain.ArrowStateReady, 30*time.Second)
			return time.Since(start)
		})

		kit.AssertNoRegression(t, name, kit.P50(durations), kit.P99(durations))
	}
}

// ─── Startup replay ───────────────────────────────────────────────────────────

// waitForHTTPCatalogLen polls GET /v0/arrow until the response contains at least
// wantLen entries or timeout elapses. Used only for startup-replay timing where
// the server does not push WS events for arrows already in stable states on new
// connections — WS cannot signal "replay complete" in that scenario.
func waitForHTTPCatalogLen(t *testing.T, env *kit.Env, wantLen int, timeout time.Duration) {
	t.Helper()
	type envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	httpClient := env.HTTPClient(5 * time.Second)
	listURL := env.URL + "/v0/arrow"
	deadline := time.Now().Add(timeout)
	for {
		resp, err := httpClient.Get(listURL) //nolint:noctx
		if err == nil {
			var e envelope
			if err2 := json.NewDecoder(resp.Body).Decode(&e); err2 == nil && len(e.Data) >= wantLen {
				resp.Body.Close()
				return
			}
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("waitForHTTPCatalogLen: timeout waiting for %d arrows", wantLen)
			return
		}
	}
}

// TestBench_StartupReplay measures the time from env creation to the HTTP catalog
// endpoint returning N arrows. This exercises event store replay and the catalog
// projection on startup.
//
// Note: the WS streams (/v0/arrow, /v0/runtime) do not emit events for arrows
// already in stable states on new connections — only crash recovery triggers WS
// events on startup. We use HTTP polling to detect readiness for this benchmark.
func TestBench_StartupReplay(t *testing.T) {
	for _, size := range []int{10, 50, 100} {
		t.Run(fmt.Sprintf("size-%d", size), func(t *testing.T) {
			home := t.TempDir()

			// ── Phase 1: populate the event store ──────────────────────────
			env1 := kit.BuildEnv(t, getRepos(t), nil, home)
			tc1 := env1.TypedClient(t)
			for i := 1; i <= size; i++ {
				ns := fmt.Sprintf("quiver.test/bench/startup-%d@v1", i)
				name := fmt.Sprintf("bench.startup-%d", i)
				seedAndWait(t, env1, tc1, ns, name)
				installAndWait(t, env1, tc1, ns)
			}
			env1.Close()

			// ── Phase 2: measure replay ────────────────────────────────────
			benchName := fmt.Sprintf("StartupReplay/arrows-%d", size)
			durations := kit.RunBenchmark(t, 5, func() time.Duration {
				start := time.Now()
				env2 := kit.BuildEnv(t, getRepos(t), nil, home)
				waitForHTTPCatalogLen(t, env2, size, 60*time.Second)
				elapsed := time.Since(start)
				env2.Close()
				return elapsed
			})

			kit.AssertNoRegression(t, benchName, kit.P50(durations), kit.P99(durations))
		})
	}
}

// ─── Event store degradation ─────────────────────────────────────────────────

// TestBench_EventStoreDegradation measures GetDetail latency after N install/uninstall
// cycles on the same namespace. Because asynx snapshots after each EndExecution,
// latency should remain stable regardless of cycle count — this benchmark validates
// that the snapshotting strategy holds under churn.
func TestBench_EventStoreDegradation(t *testing.T) {
	env, tc := newEnv(t)
	ns := "quiver.test/bench/churn@v1"
	detailURL := env.URL + "/v0/arrow/" + url.PathEscape(ns) + "/detail"
	httpClient := env.HTTPClient(10 * time.Second)

	seedAndWait(t, env, tc, ns, "bench.churn")

	measureGetDetail := func(t *testing.T, label string) {
		t.Helper()
		durations := kit.RunBenchmark(t, 30, func() time.Duration {
			start := time.Now()
			resp, _ := httpClient.Get(detailURL) //nolint:noctx
			if resp != nil {
				resp.Body.Close()
			}
			return time.Since(start)
		})
		kit.AssertNoRegression(t, label, kit.P50(durations), kit.P99(durations))
	}

	checkpoints := []int{1, 10, 50, 100}
	cyclesDone := 0

	for _, target := range checkpoints {
		for cyclesDone < target {
			installAndWait(t, env, tc, ns)
			uninstallAndWait(t, env, tc, ns)
			cyclesDone++
		}
		measureGetDetail(t, fmt.Sprintf("EventStoreDegradation/after-%d-cycles", target))
	}
}
