package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	ucmocks "github.com/rabbytesoftware/quiver/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// --- tests ---

func TestQuiverAdd_DelegatesToRepo(t *testing.T) {
	ns := domain.Namespace("test/quiver")
	called := false

	repo := &ucmocks.MockQuiver{
		AddFn: func(_ context.Context, gotNs domain.Namespace) error {
			called = true
			if gotNs != ns {
				t.Errorf("got ns=%q, want %q", gotNs, ns)
			}
			return nil
		},
	}

	uc := NewQuiverUsecase(repo)
	err := uc.Add(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected repo.Add to be called")
	}
}

func TestQuiverUpdate_DelegatesToRepo(t *testing.T) {
	ns := domain.Namespace("test/quiver")
	called := false

	repo := &ucmocks.MockQuiver{
		UpdateFn: func(_ context.Context, gotNs domain.Namespace) error {
			called = true
			if gotNs != ns {
				t.Errorf("got ns=%q, want %q", gotNs, ns)
			}
			return nil
		},
	}

	uc := NewQuiverUsecase(repo)
	err := uc.Update(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected repo.Update to be called")
	}
}

func TestQuiverRemove_DelegatesToRepo(t *testing.T) {
	ns := domain.Namespace("test/quiver")
	called := false

	repo := &ucmocks.MockQuiver{
		RemoveFn: func(_ context.Context, gotNs domain.Namespace) error {
			called = true
			if gotNs != ns {
				t.Errorf("got ns=%q, want %q", gotNs, ns)
			}
			return nil
		},
	}

	uc := NewQuiverUsecase(repo)
	err := uc.Remove(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected repo.Remove to be called")
	}
}

func TestQuiverList_DelegatesToRepo(t *testing.T) {
	ns := domain.Namespace("test/quiver")
	quivers := []domain.Quiver{
		{
			Namespace: ns,
			Manifest: domain.QuiverManifest{
				Name:        "Test Quiver",
				Description: "A test quiver",
				Tags:        []string{"test"},
			},
		},
	}

	repo := &ucmocks.MockQuiver{
		ListFn: func(_ context.Context) ([]domain.Quiver, error) {
			return quivers, nil
		},
	}

	uc := NewQuiverUsecase(repo)
	dtos, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 1 {
		t.Fatalf("expected 1 DTO, got %d", len(dtos))
	}

	dto := dtos[0]
	if dto.Namespace != ns {
		t.Errorf("got namespace=%q, want %q", dto.Namespace, ns)
	}
	if dto.Name != "Test Quiver" {
		t.Errorf("got name=%q, want %q", dto.Name, "Test Quiver")
	}
	if dto.Description != "A test quiver" {
		t.Errorf("got description=%q, want %q", dto.Description, "A test quiver")
	}
	if len(dto.Tags) != 1 || dto.Tags[0] != "test" {
		t.Errorf("got tags=%v, want %v", dto.Tags, []string{"test"})
	}
}

func TestQuiverList_ReturnsEmptyOnEmptyRepo(t *testing.T) {
	repo := &ucmocks.MockQuiver{
		ListFn: func(_ context.Context) ([]domain.Quiver, error) {
			return []domain.Quiver{}, nil
		},
	}

	uc := NewQuiverUsecase(repo)
	dtos, err := uc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 0 {
		t.Fatalf("expected 0 DTOs, got %d", len(dtos))
	}
}

func TestQuiverList_PropagatesRepoError(t *testing.T) {
	expectedErr := errors.New("repo error")

	repo := &ucmocks.MockQuiver{
		ListFn: func(_ context.Context) ([]domain.Quiver, error) {
			return nil, expectedErr
		},
	}

	uc := NewQuiverUsecase(repo)
	_, err := uc.List(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("got error %v, want %v", err, expectedErr)
	}
}

func TestQuiverGet_DelegatesToRepo(t *testing.T) {
	ns := domain.Namespace("test/quiver")
	quiver := &domain.Quiver{
		Namespace: ns,
		Manifest: domain.QuiverManifest{
			Name:        "Test Quiver",
			Description: "A test quiver",
			Tags:        []string{"test"},
		},
	}

	repo := &ucmocks.MockQuiver{
		GetFn: func(_ context.Context, gotNs domain.Namespace) (*domain.Quiver, error) {
			if gotNs != ns {
				t.Errorf("got ns=%q, want %q", gotNs, ns)
			}
			return quiver, nil
		},
	}

	uc := NewQuiverUsecase(repo)
	dto, err := uc.Get(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dto == nil {
		t.Fatal("expected non-nil DTO")
	}
	if dto.Namespace != ns {
		t.Errorf("got namespace=%q, want %q", dto.Namespace, ns)
	}
	if dto.Manifest.Name != "Test Quiver" {
		t.Errorf("got name=%q, want %q", dto.Manifest.Name, "Test Quiver")
	}
}

func TestQuiverGet_PropagatesRepoError(t *testing.T) {
	expectedErr := apperrors.ErrNotFound

	repo := &ucmocks.MockQuiver{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Quiver, error) {
			return nil, expectedErr
		},
	}

	uc := NewQuiverUsecase(repo)
	_, err := uc.Get(context.Background(), "test/quiver")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("got error %v, want %v", err, expectedErr)
	}
}
