//go:build integration

package bench_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type BenchSuite struct{ kit.IntegrationSuite }

func TestBenchmarks(t *testing.T) {
	suite.Run(t, new(BenchSuite))
}

const samples = 20

func (s *BenchSuite) TestBenchmarks_Add() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	i := 0
	d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
		i++
		ns := fmt.Sprintf("quiver.test/quiver-bench/add-%d@v1", i)
		start := time.Now()
		tc.Seed(ns, content)
		env.WaitForArrow(s.T(), ns, 30*time.Second)
		return time.Since(start)
	})
	kit.AssertNoRegression(s.T(), "BenchmarkAdd", kit.P50(d), kit.P99(d))
}

func (s *BenchSuite) TestBenchmarks_InstallCold() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")
	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, 30*time.Second)

	d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
		if tc.Uninstall(ns, nil) == http.StatusAccepted {
			env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
		}
		start := time.Now()
		s.Equal(http.StatusAccepted, tc.Install(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
		return time.Since(start)
	})
	kit.AssertNoRegression(s.T(), "BenchmarkInstallCold", kit.P50(d), kit.P99(d))
}

func (s *BenchSuite) TestBenchmarks_InstallWarm() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")
	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)

	d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
		s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
		start := time.Now()
		s.Equal(http.StatusAccepted, tc.Install(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
		return time.Since(start)
	})
	kit.AssertNoRegression(s.T(), "BenchmarkInstallWarm", kit.P50(d), kit.P99(d))
}

func (s *BenchSuite) TestBenchmarks_GetDetail() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")
	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)

	for _, concurrency := range []int{10, 50, 100} {
		concurrency := concurrency
		name := fmt.Sprintf("BenchmarkGetDetail/concurrent=%d", concurrency)
		d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
			var wg sync.WaitGroup
			durations := make([]time.Duration, concurrency)
			for i := 0; i < concurrency; i++ {
				wg.Add(1)
				i := i
				go func() {
					defer wg.Done()
					start := time.Now()
					tc.GetDetail(ns)
					durations[i] = time.Since(start)
				}()
			}
			wg.Wait()
			var max time.Duration
			for _, dur := range durations {
				if dur > max {
					max = dur
				}
			}
			return max
		})
		kit.AssertNoRegression(s.T(), name, kit.P50(d), kit.P99(d))
	}
}

func (s *BenchSuite) TestBenchmarks_List() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	seeded := 0
	for _, arrowCount := range []int{10, 100, 1000} {
		for i := 0; i < arrowCount; i++ {
			ns := fmt.Sprintf("quiver.test/quiver-bench/list-%d-%d@v1", arrowCount, i+1)
			tc.Seed(ns, content)
		}
		seeded += arrowCount
		env.WaitForCatalogLen(s.T(), seeded, 120*time.Second)

		name := fmt.Sprintf("BenchmarkList/arrows=%d", arrowCount)
		d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
			start := time.Now()
			tc.List()
			return time.Since(start)
		})
		kit.AssertNoRegression(s.T(), name, kit.P50(d), kit.P99(d))
	}
}

func (s *BenchSuite) TestBenchmarks_DepResolution() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	// deep: dep-chain/a (chain of arrows a→b→c→...→z)
	d := kit.RunBenchmark(s.T(), 3, func() time.Duration {
		start := time.Now()
		s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-chain/a", "v1")))
		s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-chain/a", "v1"), nil))
		for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
			env.WaitForState(s.T(),
				kit.NSFor(fmt.Sprintf("dep-chain/%s", string(letter)), "v1"),
				domain.ArrowStateReady, 120*time.Second)
		}
		elapsed := time.Since(start)
		for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
			ns := kit.NSFor(fmt.Sprintf("dep-chain/%s", string(letter)), "v1")
			tc.Uninstall(ns, nil)
		}
		return elapsed
	})
	kit.AssertNoRegression(s.T(), "BenchmarkDepResolution/deep", kit.P50(d), kit.P99(d))

	// wide: dep-wide/root
	dw := kit.RunBenchmark(s.T(), 3, func() time.Duration {
		start := time.Now()
		s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-wide/root", "v1")))
		s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-wide/root", "v1"), nil))
		env.WaitForState(s.T(), kit.NSFor("dep-wide/root", "v1"), domain.ArrowStateReady, 120*time.Second)
		elapsed := time.Since(start)
		tc.Uninstall(kit.NSFor("dep-wide/root", "v1"), nil)
		return elapsed
	})
	kit.AssertNoRegression(s.T(), "BenchmarkDepResolution/wide", kit.P50(dw), kit.P99(dw))
}

func (s *BenchSuite) TestBenchmarks_StartupCatalogReplay() {
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	for _, arrowCount := range []int{10, 100, 500} {
		home := s.T().TempDir()
		env1 := s.NewEnvWithHome(home)
		tc1 := env1.TypedClient(s.T())
		for i := 0; i < arrowCount; i++ {
			ns := fmt.Sprintf("quiver.test/quiver-bench/startup-%d-%d@v1", arrowCount, i+1)
			tc1.Seed(ns, content)
		}
		env1.WaitForCatalogLen(s.T(), arrowCount, 120*time.Second)
		env1.Close()

		name := fmt.Sprintf("BenchmarkStartupCatalogReplay/arrows=%d", arrowCount)
		d := kit.RunBenchmark(s.T(), 3, func() time.Duration {
			start := time.Now()
			env2 := kit.BuildEnv(s.T(), s.Repos, home)
			env2.TypedClient(s.T()).List()
			elapsed := time.Since(start)
			env2.Close()
			return elapsed
		})
		kit.AssertNoRegression(s.T(), name, kit.P50(d), kit.P99(d))
	}
}

func (s *BenchSuite) TestBenchmarks_EventStoreReplayDegradation() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")
	s.Equal(http.StatusCreated, tc.Add(ns))

	for _, cycles := range []int{1, 10, 100} {
		for c := 0; c < cycles; c++ {
			s.Equal(http.StatusAccepted, tc.Install(ns, nil))
			env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
			s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
			env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
		}

		name := fmt.Sprintf("BenchmarkEventStoreReplayDegradation/cycles=%d", cycles)
		d := kit.RunBenchmark(s.T(), samples, func() time.Duration {
			start := time.Now()
			tc.GetDetail(ns)
			return time.Since(start)
		})
		kit.AssertNoRegression(s.T(), name, kit.P50(d), kit.P99(d))
	}
}
