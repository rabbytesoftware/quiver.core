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
