package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestContainerNew_OnRuntimeEndedError(t *testing.T) {
	expected := errors.New("subscribe error")
	mockRT := &ucmocks.MockRuntime{
		OnRuntimeEndedFn: func(fn func(context.Context, domainRuntime.ArrowRuntime)) error {
			return expected
		},
	}
	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    mockRT,
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}
	if _, err := New(repos, nil, nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestContainerNew_OnArrowUpgradedError(t *testing.T) {
	expected := errors.New("upgraded subscribe error")
	mockRT := &ucmocks.MockRuntime{}
	mockArrow := &ucmocks.MockArrow{
		OnArrowUpgradedFn: func(fn func(context.Context, domain.Arrow) error) error {
			return expected
		},
	}
	repos := &repositories.Container{
		Arrow:      mockArrow,
		Runtime:    mockRT,
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}
	if _, err := New(repos, nil, nil); !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestContainerNew_WiresOnRuntimeEnded(t *testing.T) {
	callbackWired := false
	mockRT := &ucmocks.MockRuntime{
		OnRuntimeEndedFn: func(fn func(context.Context, domainRuntime.ArrowRuntime)) error {
			callbackWired = true
			return nil
		},
	}

	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    mockRT,
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}

	container, err := New(repos, nil, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if !callbackWired {
		t.Error("OnRuntimeEnded callback was not wired")
	}

	if container == nil {
		t.Fatal("container is nil")
	}

	if container.Arrow == nil {
		t.Error("container.Arrow is nil")
	}

	if container.Runtime == nil {
		t.Error("container.Runtime is nil")
	}

	if container.Collection == nil {
		t.Error("container.Collection is nil")
	}
}
