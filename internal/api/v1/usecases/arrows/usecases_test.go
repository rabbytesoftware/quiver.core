package arrows

import (
    "testing"

    "github.com/rabbytesoftware/quiver/internal/infrastructure"
    "github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewApiArrowsUsecases_FieldsSet(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)

    uc := NewApiArrowsUsecases(repos)
    if uc == nil {
        t.Fatal("NewApiArrowsUsecases returned nil")
    }

    if uc.repository == nil {
        t.Fatal("expected repository to be set")
    }

    if uc.ctx == nil {
        t.Fatal("expected ctx to be set")
    }
}

func TestAdd_InvalidType(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)

    uc := NewApiArrowsUsecases(repos)

    a, warns, err := uc.Add("value", "invalid", "127.0.0.1")
    if err == nil {
        t.Fatalf("expected error for invalid type, got nil; a=%v warns=%v", a, warns)
    }
}

func TestAdd_UrlSucceeds(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)

    uc := NewApiArrowsUsecases(repos)

    a, warns, err := uc.Add("http://example.com/arrow", "url", "127.0.0.1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if a == nil {
        t.Fatal("expected arrow pointer, got nil")
    }
    _ = warns
}

func TestRemove_List_Get_NoErrors(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)

    uc := NewApiArrowsUsecases(repos)

    if warns, err := uc.Remove("some-namespace", "127.0.0.1"); err != nil {
        t.Fatalf("Remove returned error: %v warns: %v", err, warns)
    }

    if _, warns, err := uc.List(); err != nil {
        t.Fatalf("List returned error: %v warns: %v", err, warns)
    }

    if a, warns, err := uc.Get("some-namespace"); err != nil {
        t.Fatalf("Get returned error: %v warns: %v", err, warns)
    } else if a == nil {
        t.Fatalf("Get returned nil arrow pointer; warns=%v", warns)
    }
}

