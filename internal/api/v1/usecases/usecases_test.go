package usecases

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewApiUsecases_FieldsSet(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)

    uc := NewApiUsecases(repos)
    if uc == nil {
        t.Fatal("NewApiUsecases returned nil")
    }

    if uc.Arrows == nil {
        t.Fatal("expected Arrows to be set")
    }
    if uc.Quivers == nil {
        t.Fatal("expected Quivers to be set")
    }
    if uc.System == nil {
        t.Fatal("expected System to be set")
    }
}

func TestArrows_List_DelegatesToRepository(t *testing.T) {
    infra := infrastructure.NewInfrastructure()
    repos := repositories.NewRepositories(infra)
		ctx := context.Background()

    uc := NewApiUsecases(repos)

    gotMap, gotWarns, gotErr := uc.Arrows.List()
    wantMap, wantWarns, wantErr := repos.GetArrows().List(ctx)

    if (gotErr != nil) != (wantErr != nil) {
        t.Fatalf("List error mismatch: gotErr=%v wantErr=%v", gotErr, wantErr)
    }

    if len(gotWarns) != len(wantWarns) {
        t.Fatalf("List warns length mismatch: got=%d want=%d", len(gotWarns), len(wantWarns))
    }

    if len(gotMap) != len(wantMap) {
        t.Fatalf("List map length mismatch: got=%d want=%d", len(gotMap), len(wantMap))
    }
}

