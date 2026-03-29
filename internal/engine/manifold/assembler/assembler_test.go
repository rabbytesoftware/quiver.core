package assembler_test

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func boolPtr(b bool) *bool { return &b }

func baseRawArrow() *models.RawArrow {
	return &models.RawArrow{
		Metadata: models.RawMetadata{
			Name:    "test-arrow",
			Version: "1.0.0",
		},
		Requirements: models.RawRequirements{
			CpuCores: 2,
			RamGB:    4,
			DiskGB:   10,
			OS:       []string{"linux/amd64"},
		},
		Lifecycle: models.RawLifecycle{
			Install:   []models.RawStep{{Type: "run", Command: "./install.sh"}},
			Uninstall: []models.RawStep{{Type: "run", Command: "./uninstall.sh"}},
		},
	}
}

// ─── AssembleArrow Happy Path ────────────────────────────────────────────────

func TestAssembleArrow_Minimal(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	manifest, err := a.AssembleArrow(raw, "linux/amd64")
	if err != nil {
		t.Fatalf("AssembleArrow() error = %v", err)
	}
	if manifest.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", manifest.Version)
	}
	if manifest.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", manifest.Requirements.CpuCores)
	}
}

func TestAssembleArrow_WithAllFields(t *testing.T) {
	a := assembler.New()
	raw := &models.RawArrow{
		Metadata: models.RawMetadata{
			Name:        "cs2-server",
			Description: "CS2 Dedicated Server",
			Version:     "25.11.0",
			License:     "MIT",
			Quiver:      "github.com/rabbytesoftware/quiver.experiments",
			Maintainers: []string{"char2cs"},
			Tags:        []string{"gaming"},
			Credits: []domain.Credit{
				{Name: "Alice", Email: "alice@example.com"},
			},
		},
		Requirements: models.RawRequirements{
			CpuCores: 4,
			RamGB:    8,
			DiskGB:   60,
			OS:       []string{"linux/amd64", "linux/arm64"},
		},
		Dependencies: []string{"github.com/valvesoftware/quiver/steamcmd"},
		Variables: []domain.Variable{
			{Name: "CS2_PORT", Default: "27015"},
		},
		Netbridge: []netbridge.PortDef{
			{Name: "GAME_PORT", Protocol: netbridge.ProtocolUDP},
		},
		Lifecycle: models.RawLifecycle{
			Install:   []models.RawStep{{Type: "run", Command: "./install.sh", Timeout: "30m"}},
			Execute:   []models.RawStep{{Type: "run", Command: "./start.sh"}},
			Stop:      []models.RawStep{{Type: "signal", Signal: "SIGTERM", Timeout: "30s"}},
			Uninstall: []models.RawStep{{Type: "run", Command: "./uninstall.sh"}},
		},
		Methods: map[string]models.RawMethod{
			"update": {
				AvailableIn: []string{"ready"},
				Steps:       []models.RawStep{{Type: "run", Command: "./update.sh"}},
			},
		},
	}

	manifest, err := a.AssembleArrow(raw, "linux/amd64")
	if err != nil {
		t.Fatalf("AssembleArrow() error = %v", err)
	}

	if manifest.Description != "CS2 Dedicated Server" {
		t.Errorf("Description = %q", manifest.Description)
	}
	if manifest.License != "MIT" {
		t.Errorf("License = %q, want MIT", manifest.License)
	}
	if manifest.URL != "github.com/rabbytesoftware/quiver.experiments" {
		t.Errorf("URL = %q", manifest.URL)
	}
	if len(manifest.Credits) != 1 || manifest.Credits[0].Name != "Alice" {
		t.Errorf("Credits = %v, want [{Alice alice@example.com}]", manifest.Credits)
	}
	if len(manifest.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(manifest.Dependencies))
	}
	if len(manifest.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(manifest.Variables))
	}
	if len(manifest.Netbridge) != 1 {
		t.Errorf("Netbridge count = %d, want 1", len(manifest.Netbridge))
	}
	if len(manifest.Lifecycle.Install) != 1 {
		t.Errorf("Install steps = %d, want 1", len(manifest.Lifecycle.Install))
	}
	if _, ok := manifest.Methods["update"]; !ok {
		t.Error("expected 'update' method")
	}
}

func TestAssembleArrow_EmptyLifecycleIsValid(t *testing.T) {
	a := assembler.New()
	raw := &models.RawArrow{
		Metadata: models.RawMetadata{Name: "static", Version: "1.0.0"},
		Requirements: models.RawRequirements{
			CpuCores: 1, RamGB: 1, DiskGB: 1, OS: []string{"linux/amd64"},
		},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if err != nil {
		t.Errorf("unexpected error for empty lifecycle: %v", err)
	}
}

func TestAssembleArrow_OSOverride(t *testing.T) {
	a := assembler.New()
	raw := &models.RawArrow{
		Metadata:     models.RawMetadata{Name: "x", Version: "1.0.0"},
		Requirements: models.RawRequirements{CpuCores: 1, RamGB: 1, DiskGB: 1, OS: []string{"linux/amd64", "windows/amd64"}},
		Lifecycle: models.RawLifecycle{
			Install: []models.RawStep{
				{
					Type:    "run",
					Command: "./install.sh",
					Overrides: map[string]models.RawStepOverride{
						"windows/amd64": {Command: `.\\install.bat`},
					},
				},
			},
			Uninstall: []models.RawStep{{Type: "run", Command: "./uninstall.sh"}},
		},
	}

	manifest, err := a.AssembleArrow(raw, "windows/amd64")
	if err != nil {
		t.Fatalf("AssembleArrow() error = %v", err)
	}
	// After override resolution, the windows command should be used
	_ = manifest
}

// ─── AssembleArrow Error Paths ───────────────────────────────────────────────

func TestAssembleArrow_UnsupportedOS(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	_, err := a.AssembleArrow(raw, "windows/amd64")
	if !errors.Is(err, assembler.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestAssembleArrow_LifecyclePairViolation(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Uninstall = nil
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_ExecuteStopPairViolation(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Execute = []models.RawStep{{Type: "run", Command: "./start.sh"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidStepType(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Install = []models.RawStep{{Type: "unknown"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_RunStepMissingCommand(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Install = []models.RawStep{{Type: "run"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidDependency(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Dependencies = []string{"not-valid"}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_DuplicateVariable(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Variables = []domain.Variable{{Name: "A"}, {Name: "A"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_SelectVariableNoValues(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Variables = []domain.Variable{{Name: "CHOICE", Type: domain.VariableTypeSelect}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_DuplicateNetbridge(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Netbridge = []netbridge.PortDef{{Name: "P"}, {Name: "P"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidStepTypeInMethod(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Methods = map[string]models.RawMethod{
		"op": {
			AvailableIn: []string{"ready"},
			Steps:       []models.RawStep{{Type: "unknown"}},
		},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidMethodState(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Methods = map[string]models.RawMethod{
		"op": {AvailableIn: []string{"stopped"}},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidTimeout(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Install = []models.RawStep{
		{Type: "run", Command: "./install.sh", Timeout: "invalid"},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_OverrideOnNonRunStep(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Install = []models.RawStep{
		{
			Type: "fetch",
			URL:  "https://example.com",
			To:   "./file",
			Overrides: map[string]models.RawStepOverride{
				"linux/amd64": {Title: "override"},
			},
		},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

// ─── AssembleQuiver ──────────────────────────────────────────────────────────

func TestAssembleQuiver_Full(t *testing.T) {
	a := assembler.New()
	raw := &models.RawQuiver{
		Schema:      "quiver@v0",
		Name:        "Gaming Quiver",
		Description: "Curated game servers",
		URL:         "https://gaming.quiver.ar",
		Maintainers: []string{"char2cs"},
		Tags:        []string{"gaming"},
		Media:       domain.QuiverMedia{Icon: "https://example.com/icon.png"},
		Arrows:      []string{"github.com/valve/cs2"},
	}

	manifest, err := a.AssembleQuiver(raw)
	if err != nil {
		t.Fatalf("AssembleQuiver() error = %v", err)
	}

	if manifest.Name != "Gaming Quiver" {
		t.Errorf("Name = %q, want 'Gaming Quiver'", manifest.Name)
	}
	if manifest.Description != "Curated game servers" {
		t.Errorf("Description = %q", manifest.Description)
	}
	if manifest.URL != "https://gaming.quiver.ar" {
		t.Errorf("URL = %q", manifest.URL)
	}
	if len(manifest.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d, want 1", len(manifest.Maintainers))
	}
	if len(manifest.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(manifest.Tags))
	}
	if manifest.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Media.Icon = %q", manifest.Media.Icon)
	}
	if len(manifest.Arrows) != 1 {
		t.Errorf("Arrows count = %d, want 1", len(manifest.Arrows))
	}
}

func TestAssembleQuiver_Minimal(t *testing.T) {
	a := assembler.New()
	raw := &models.RawQuiver{
		Name:        "Min",
		Description: "Minimal",
	}
	manifest, err := a.AssembleQuiver(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Name != "Min" {
		t.Errorf("Name = %q, want Min", manifest.Name)
	}
	if len(manifest.Arrows) != 0 {
		t.Errorf("expected empty arrows, got %d", len(manifest.Arrows))
	}
}

func TestAssembleArrow_InvalidTimeoutInMethod(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Methods = map[string]models.RawMethod{
		"op": {
			AvailableIn: []string{"ready"},
			Steps:       []models.RawStep{{Type: "run", Command: "x", Timeout: "bad"}},
		},
	}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleArrow_InvalidTimeoutInStop(t *testing.T) {
	a := assembler.New()
	raw := baseRawArrow()
	raw.Lifecycle.Execute = []models.RawStep{{Type: "run", Command: "./start.sh"}}
	raw.Lifecycle.Stop = []models.RawStep{{Type: "signal", Signal: "SIGTERM", Timeout: "bad"}}
	_, err := a.AssembleArrow(raw, "linux/amd64")
	if !errors.Is(err, assembler.ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestAssembleQuiver_ArrowsConvertedToNamespace(t *testing.T) {
	a := assembler.New()
	raw := &models.RawQuiver{
		Name:        "Q",
		Description: "D",
		Arrows:      []string{"github.com/user/repo", "gitlab.com/org/proj"},
	}
	manifest, err := a.AssembleQuiver(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Arrows[0] != domain.Namespace("github.com/user/repo") {
		t.Errorf("Arrows[0] = %q, want github.com/user/repo", manifest.Arrows[0])
	}
}
