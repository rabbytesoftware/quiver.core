package arrows

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewArrowsUsecase(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecase := NewArrowsUsecase(repos)

	if usecase == nil {
		t.Fatal("NewArrowsUsecase() returned nil")
	}
}

func TestArrowsUsecaseStructure(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecase := NewArrowsUsecase(repos)

	// Test that repositories is properly set
	if usecase.repositories == nil {
		t.Error("ArrowsUsecase.repositories is nil")
	}

	if usecase.repositories != repos {
		t.Error("ArrowsUsecase.repositories is not the same instance passed to constructor")
	}
}

func TestArrowsUsecaseWithNilRepositories(t *testing.T) {
	// Test behavior when nil repositories is passed
	usecase := NewArrowsUsecase(nil)

	if usecase == nil {
		t.Fatal("NewArrowsUsecase(nil) returned nil")
	}

	// Test that repositories field is set to nil
	if usecase.repositories != nil {
		t.Error("ArrowsUsecase.repositories should be nil when nil is passed to constructor")
	}
}

func TestMultipleArrowsUsecaseInstances(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	usecase1 := NewArrowsUsecase(repos)
	usecase2 := NewArrowsUsecase(repos)

	// Different ArrowsUsecase instances
	if usecase1 == usecase2 {
		t.Error("NewArrowsUsecase() should create new instances each time")
	}

	// But they should reference the same repositories instance
	if usecase1.repositories != usecase2.repositories {
		t.Error("ArrowsUsecase instances should reference the same repositories when created with the same repositories")
	}
}

func TestArrowsUsecaseFieldAccess(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecase := NewArrowsUsecase(repos)

	// Test that we can access the repositories field
	repositories := usecase.repositories
	if repositories == nil {
		t.Error("Cannot access repositories field")
	}

	if repositories != repos {
		t.Error("repositories field does not match the instance passed to constructor")
	}

	// Test that accessing the same field returns the same instance
	repositories2 := usecase.repositories
	if repositories != repositories2 {
		t.Error("repositories field should return the same instance")
	}
}

func TestArrowsUsecaseType(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecase := NewArrowsUsecase(repos)

	// Verify the usecase is not nil
	if usecase == nil {
		t.Error("NewArrowsUsecase() returned nil")
	}
}

func TestArrowsUsecaseZeroValue(t *testing.T) {
	// Test that a zero-value ArrowsUsecase struct is valid
	var usecase ArrowsUsecase

	// Test that we can take a pointer to it
	usecasePtr := &usecase
	_ = usecasePtr // Pointer to local variable is never nil

	// Test that repositories field is nil in zero value
	if usecase.repositories != nil {
		t.Error("Zero-value ArrowsUsecase should have nil repositories")
	}
}
