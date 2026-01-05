package quivers

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewApiQuiversUsecases_FieldsSet(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)
	if uc == nil {
		t.Fatal("NewApiQuiversUsecases returned nil")
	}

	if uc.rp == nil {
		t.Fatal("expected rp to be set")
	}

	if uc.ctx == nil {
		t.Fatal("expected ctx to be set")
	}
}

func TestNewApiQuiversUsecases_RPMethods(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)

	got := uc.rp.Get()
	want := repos.GetQuivers().Get()

	if len(got) != len(want) {
		t.Fatalf("rp.Get() length mismatch: got %d want %d", len(got), len(want))
	}
}
