package assembler

import (
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestBuildStep_Run(t *testing.T) {
	a := New()
	s, err := a.buildStep(models.RawStep{
		Type:    "run",
		Command: "echo hello",
		Title:   "Say hello",
		Timeout: "10s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type() != step.StepTypeRun {
		t.Errorf("Type = %v, want run", s.Type())
	}
	if s.Title() != "Say hello" {
		t.Errorf("Title = %q, want Say hello", s.Title())
	}
	run, ok := s.(step.RunStep)
	if !ok {
		t.Fatal("expected RunStep")
	}
	if run.Command != "echo hello" {
		t.Errorf("Command = %q, want echo hello", run.Command)
	}
	if run.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", run.Timeout)
	}
}

func TestBuildStep_Fetch(t *testing.T) {
	a := New()
	s, err := a.buildStep(models.RawStep{
		Type:  "fetch",
		URL:   "https://example.com/file",
		To:    "./file",
		Title: "Downloading",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type() != step.StepTypeFetch {
		t.Errorf("Type = %v, want fetch", s.Type())
	}
	fetch, ok := s.(step.FetchStep)
	if !ok {
		t.Fatal("expected FetchStep")
	}
	if fetch.URL != "https://example.com/file" {
		t.Errorf("URL = %q, want https://example.com/file", fetch.URL)
	}
	if fetch.To != "./file" {
		t.Errorf("To = %q, want ./file", fetch.To)
	}
}

func TestBuildStep_Signal(t *testing.T) {
	a := New()
	s, err := a.buildStep(models.RawStep{
		Type:    "signal",
		Signal:  "SIGTERM",
		Timeout: "30s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type() != step.StepTypeSignal {
		t.Errorf("Type = %v, want signal", s.Type())
	}
	sig, ok := s.(step.SignalStep)
	if !ok {
		t.Fatal("expected SignalStep")
	}
	if sig.Signal != "SIGTERM" {
		t.Errorf("Signal = %q, want SIGTERM", sig.Signal)
	}
}

func TestBuildStep_Dependencies(t *testing.T) {
	a := New()
	s, err := a.buildStep(models.RawStep{
		Type:  "dependencies",
		Title: "Install deps",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type() != step.StepTypeDependencies {
		t.Errorf("Type = %v, want dependencies", s.Type())
	}
	if !s.ExitOnFailure() {
		t.Error("DependenciesStep should always exit on failure")
	}
}

func TestBuildStep_ExitOnFailure_Default(t *testing.T) {
	a := New()
	s, err := a.buildStep(models.RawStep{Type: "run", Command: "echo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.ExitOnFailure() {
		t.Error("default ExitOnFailure should be true")
	}
}

func TestBuildStep_ExitOnFailure_False(t *testing.T) {
	a := New()
	f := false
	s, err := a.buildStep(models.RawStep{
		Type:          "run",
		Command:       "echo",
		ExitOnFailure: &f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ExitOnFailure() {
		t.Error("ExitOnFailure should be false")
	}
}

func TestBuildSteps_Empty(t *testing.T) {
	a := New()
	steps, err := a.buildSteps(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected empty slice, got %d steps", len(steps))
	}
}

func TestBuildSteps_Multiple(t *testing.T) {
	a := New()
	raw := []models.RawStep{
		{Type: "run", Command: "a"},
		{Type: "run", Command: "b"},
		{Type: "signal", Signal: "SIGTERM"},
	}
	steps, err := a.buildSteps(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 3 {
		t.Errorf("len = %d, want 3", len(steps))
	}
}

func TestBuildLifecycle(t *testing.T) {
	a := New()
	lc := models.RawLifecycle{
		Install:   []models.RawStep{{Type: "run", Command: "install.sh"}},
		Execute:   []models.RawStep{{Type: "run", Command: "start.sh"}},
		Stop:      []models.RawStep{{Type: "signal", Signal: "SIGTERM"}},
		Uninstall: []models.RawStep{{Type: "run", Command: "uninstall.sh"}},
	}
	built, err := a.buildLifecycle(lc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(built.Install) != 1 {
		t.Errorf("Install len = %d, want 1", len(built.Install))
	}
	if len(built.Execute) != 1 {
		t.Errorf("Execute len = %d, want 1", len(built.Execute))
	}
	if len(built.Stop) != 1 {
		t.Errorf("Stop len = %d, want 1", len(built.Stop))
	}
	if len(built.Uninstall) != 1 {
		t.Errorf("Uninstall len = %d, want 1", len(built.Uninstall))
	}
}

func TestBuildMethods(t *testing.T) {
	a := New()
	raw := map[string]models.RawMethod{
		"update": {
			AvailableIn: []string{"ready"},
			Steps:       []models.RawStep{{Type: "run", Command: "update.sh"}},
		},
	}
	methods, err := a.buildMethods(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := methods["update"]
	if !ok {
		t.Fatal("expected 'update' method")
	}
	if len(m.AvailableIn) != 1 {
		t.Errorf("AvailableIn len = %d, want 1", len(m.AvailableIn))
	}
	if m.AvailableIn[0] != domain.ArrowStateReady {
		t.Errorf("AvailableIn[0] = %q, want ready", m.AvailableIn[0])
	}
	if len(m.Steps) != 1 {
		t.Errorf("Steps len = %d, want 1", len(m.Steps))
	}
}

func TestBuildRequirement(t *testing.T) {
	raw := models.RawRequirements{
		CpuCores: 4,
		RamGB:    8,
		DiskGB:   60,
		OS:       []string{"linux/amd64", "darwin/amd64"},
	}
	req, err := buildRequirement(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.CpuCores != 4 {
		t.Errorf("CpuCores = %d, want 4", req.CpuCores)
	}
	if req.MemoryGB != 8 {
		t.Errorf("MemoryGB = %d, want 8", req.MemoryGB)
	}
	if req.DiskGB != 60 {
		t.Errorf("DiskGB = %d, want 60", req.DiskGB)
	}
	if len(req.OS) != 2 {
		t.Errorf("OS count = %d, want 2", len(req.OS))
	}
	if req.OS[0] != domain.OSLinuxAMD64 {
		t.Errorf("OS[0] = %q, want linux/amd64", req.OS[0])
	}
}

func TestBuildRequirement_ValidationFailure(t *testing.T) {
	raw := models.RawRequirements{
		CpuCores: 0,
		RamGB:    0,
		DiskGB:   0,
	}
	_, err := buildRequirement(raw)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestParseDuration_Empty(t *testing.T) {
	d, err := parseDuration("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 0 {
		t.Errorf("d = %v, want 0", d)
	}
}

func TestParseDuration_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"10m", 10 * time.Minute},
		{"1h", time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			d, err := parseDuration(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d != tc.want {
				t.Errorf("d = %v, want %v", d, tc.want)
			}
		})
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	_, err := parseDuration("not-a-duration")
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestBuildLifecycle_ErrorInExecute(t *testing.T) {
	a := New()
	lc := models.RawLifecycle{
		Execute: []models.RawStep{{Type: "run", Command: "x", Timeout: "bad"}},
	}
	_, err := a.buildLifecycle(lc)
	if err == nil {
		t.Error("expected error for invalid execute timeout")
	}
}

func TestBuildLifecycle_ErrorInStop(t *testing.T) {
	a := New()
	lc := models.RawLifecycle{
		Stop: []models.RawStep{{Type: "signal", Signal: "SIGTERM", Timeout: "bad"}},
	}
	_, err := a.buildLifecycle(lc)
	if err == nil {
		t.Error("expected error for invalid stop timeout")
	}
}

func TestBuildLifecycle_ErrorInUninstall(t *testing.T) {
	a := New()
	lc := models.RawLifecycle{
		Uninstall: []models.RawStep{{Type: "run", Command: "x", Timeout: "bad"}},
	}
	_, err := a.buildLifecycle(lc)
	if err == nil {
		t.Error("expected error for invalid uninstall timeout")
	}
}

func TestBuildLifecycle_ErrorInInstall(t *testing.T) {
	a := New()
	lc := models.RawLifecycle{
		Install: []models.RawStep{{Type: "run", Command: "x", Timeout: "bad"}},
	}
	_, err := a.buildLifecycle(lc)
	if err == nil {
		t.Error("expected error for invalid install timeout")
	}
}

func TestBuildMethods_ErrorInSteps(t *testing.T) {
	a := New()
	raw := map[string]models.RawMethod{
		"op": {
			AvailableIn: []string{"ready"},
			Steps:       []models.RawStep{{Type: "run", Command: "x", Timeout: "bad"}},
		},
	}
	_, err := a.buildMethods(raw)
	if err == nil {
		t.Error("expected error for invalid step timeout in method")
	}
}

func TestResolveExitOnFailure_Nil(t *testing.T) {
	if !resolveExitOnFailure(nil) {
		t.Error("nil should default to true")
	}
}

func TestResolveExitOnFailure_True(t *testing.T) {
	b := true
	if !resolveExitOnFailure(&b) {
		t.Error("true should return true")
	}
}

func TestResolveExitOnFailure_False(t *testing.T) {
	b := false
	if resolveExitOnFailure(&b) {
		t.Error("false should return false")
	}
}
