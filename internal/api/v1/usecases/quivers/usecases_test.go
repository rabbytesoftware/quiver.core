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

func TestApiQuiversUsecases_GetById(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)

	// Just verify the method doesn't panic
	result := uc.rp.GetById("test-id")
	_ = result
}

func TestApiQuiversUsecases_Create(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.Create(nil)
}

func TestApiQuiversUsecases_Update(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.Update(nil)
}

func TestApiQuiversUsecases_DeleteById(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiQuiversUsecases(repos)

	// Just verify the method doesn't panic
	uc.rp.DeleteById("test-id")
}
