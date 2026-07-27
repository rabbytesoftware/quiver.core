//go:build integration

package crash_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type CrashSuite struct{ kit.IntegrationSuite }

func TestCrashIntegration(t *testing.T) {
	suite.Run(t, new(CrashSuite))
}

// TestCrash_MidInstall_Recovery: crash while tool-a-slow is installing.
// tool-a-slow has a 1s sleep in its install step, giving us time to close.
// Expected recovery: absent (install never completed).
func (s *CrashSuite) TestCrash_MidInstall_Recovery() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/tool-a-slow", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/tool-a-slow", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a-slow", "v1"), domain.ArrowStateInstalling, 120*time.Second)
	env1.CloseCrashing()

	env2 := s.NewEnvWithHome(home)
	// WaitForState waits for the runtime.recovered WS event — ensuring recovery used
	// RecoverInterrupted (not Forget). If Forget were called the runtime would be gone
	// and no WS event would arrive, causing a timeout here.
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a-slow", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env2.Close()
}

// TestCrash_MidUninstall_Recovery: crash while tool-a-slow is uninstalling.
// tool-a-slow has a 1s sleep in its uninstall step.
// Expected recovery: absent (partial uninstall = treat as not installed).
func (s *CrashSuite) TestCrash_MidUninstall_Recovery() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/tool-a-slow", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/tool-a-slow", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a-slow", "v1"), domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Uninstall(kit.NSFor("quiver-test/tool-a-slow", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a-slow", "v1"), domain.ArrowStateUninstalling, 120*time.Second)
	env1.CloseCrashing()

	env2 := s.NewEnvWithHome(home)
	// WaitForState waits for the runtime.recovered WS event — ensuring recovery used
	// RecoverInterrupted (not Forget). If Forget were called the runtime would be gone
	// and no WS event would arrive, causing a timeout here.
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a-slow", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env2.Close()
}

// TestCrash_MidUpdate_Recovery: crash while tool-with-update is updating.
// tool-with-update has a 1s sleep in its update step.
// Expected recovery: absent (partial update = unsafe; needs reinstall).
func (s *CrashSuite) TestCrash_MidUpdate_Recovery() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/tool-with-update", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/tool-with-update", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-with-update", "v1"), domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Execute(kit.NSFor("quiver-test/tool-with-update", "v1"), "_update", nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-with-update", "v1"), domain.ArrowStateUpdating, 120*time.Second)
	env1.CloseCrashing()

	env2 := s.NewEnvWithHome(home)
	// WaitForState waits for the runtime.recovered WS event — ensuring recovery used
	// RecoverInterrupted (not Forget). If Forget were called the runtime would be gone
	// and no WS event would arrive, causing a timeout here.
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/tool-with-update", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env2.Close()
}

// TestCrash_Running_AlivePID_Recovery: env closes while service-running is in running state
// but WITHOUT killing the OS process (CloseWithoutKilling).
// env2 opens with the same home — recoverTransients detects the PID is alive → detached.
func (s *CrashSuite) TestCrash_Running_AlivePID_Recovery() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/service-running", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/service-running", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Execute(kit.NSFor("quiver-test/service-running", "v1"), "start", nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env1.WaitForActivePID(s.T(), kit.NSFor("quiver-test/service-running", "v1"), 120*time.Second)
	env1.CloseWithoutKilling()

	env2 := s.NewEnvWithHome(home)
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateDetached, 120*time.Second)
	env2.Close()
}

// TestCrash_Detached_StopTerminatesProcess: env closes without killing a running process (detached).
// env2 opens, finds the process detached, then calls Stop → process is killed → ready.
func (s *CrashSuite) TestCrash_Detached_StopTerminatesProcess() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/service-running", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/service-running", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Execute(kit.NSFor("quiver-test/service-running", "v1"), "start", nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env1.WaitForActivePID(s.T(), kit.NSFor("quiver-test/service-running", "v1"), 120*time.Second)
	env1.CloseWithoutKilling()

	env2 := s.NewEnvWithHome(home)
	tc2 := env2.TypedClient(s.T())
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateDetached, 120*time.Second)

	s.Equal(http.StatusAccepted, tc2.Stop(kit.NSFor("quiver-test/service-running", "v1")))
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/service-running", "v1"), domain.ArrowStateReady, 120*time.Second)
	env2.Close()
}

// TestCrash_SIGTERMDuringInstall: graceful shutdown (SIGTERM equivalent) while
// tool-a-slow is mid-install. env.Close() calls appContainer.Shutdown which
// drains via drainWg. Arrow must NOT be stuck in installing on restart.
func (s *CrashSuite) TestCrash_SIGTERMDuringInstall() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a-slow", "v1")
	s.Equal(http.StatusCreated, tc1.Add(ns))
	env1.WaitForArrow(s.T(), ns, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(ns, nil))
	env1.WaitForState(s.T(), ns, domain.ArrowStateInstalling, 120*time.Second)
	env1.Close() // graceful shutdown — drainWg waits for install to finish or abort

	env2 := s.NewEnvWithHome(home)
	detail, status := env2.TypedClient(s.T()).GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.NotEqual(string(domain.ArrowStateInstalling), detail.State,
		"arrow must not be stuck in installing after graceful shutdown, got: %s", detail.State)
	env2.Close()
}

// TestCrash_MidStop_Recovery: crash while service-stopping-slow is stopping (1s sleep in stop step).
// Expected recovery: ready (partial stop = binaries still intact; wizard never recorded completion).
func (s *CrashSuite) TestCrash_MidStop_Recovery() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/service-stopping-slow", "v1")))
	env1.WaitForListLen(s.T(), 1, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/service-stopping-slow", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-stopping-slow", "v1"), domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Execute(kit.NSFor("quiver-test/service-stopping-slow", "v1"), "execute", nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-stopping-slow", "v1"), domain.ArrowStateRunning, 120*time.Second)
	s.Equal(http.StatusAccepted, tc1.Stop(kit.NSFor("quiver-test/service-stopping-slow", "v1")))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/service-stopping-slow", "v1"), domain.ArrowStateStopping, 120*time.Second)
	env1.CloseCrashing()

	env2 := s.NewEnvWithHome(home)
	// WaitForState waits for the runtime.recovered WS event — ensuring recovery used
	// RecoverInterrupted (not Forget). If Forget were called the runtime would be gone
	// and no WS event would arrive, causing a timeout here.
	env2.WaitForState(s.T(), kit.NSFor("quiver-test/service-stopping-slow", "v1"), domain.ArrowStateReady, 120*time.Second)
	env2.Close()
}
