package system

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewApiSystemUsecases_FieldsSet(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)
	if uc == nil {
		t.Fatal("NewApiSystemUsecases returned nil")
	}

	if uc.rp == nil {
		t.Fatal("expected rp to be set")
	}

	if uc.ctx == nil {
		t.Fatal("expected ctx to be set")
	}
}

func TestNewApiSystemUsecases_RPMethods(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Call a method on rp and compare to the repository returned by repos.GetSystem()
	got := uc.rp.Status()
	want := repos.GetSystem().Status()

	if got != want {
		t.Fatalf("rp.Status() mismatch: got %q want %q", got, want)
	}
}

func TestApiSystemUsecases_GetMetadata(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	metadata := uc.rp.GetMetadata()
	_ = metadata
}

func TestApiSystemUsecases_UpdateQuiver(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.UpdateQuiver()
}

func TestApiSystemUsecases_UninstallQuiver(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.UninstallQuiver()
}

func TestApiSystemUsecases_GetLogs(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	logs := uc.rp.GetLogs()
	_ = logs
}

func TestApiSystemUsecases_RestartQuiver(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.RestartQuiver()
}

func TestApiSystemUsecases_StopQuiver(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiSystemUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.StopQuiver()
}
