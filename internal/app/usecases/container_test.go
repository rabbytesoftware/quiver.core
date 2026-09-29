package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// An update advances its row in place, so nothing reacts to the legacy
// successor-row upgrade event any more.
func TestContainerNew_DoesNotReactToArrowUpgrades(t *testing.T) {
	subscribed := false
	mockArrow := &ucmocks.MockArrow{
		OnArrowUpgradedFn: func(func(context.Context, domain.Arrow) error) error {
			subscribed = true
			return nil
		},
	}
	repos := &repositories.Container{
		Arrow:      mockArrow,
		Runtime:    &ucmocks.MockRuntime{},
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}
	_, err := New(repos, nil, nil)
	require.NoError(t, err)
	assert.False(t, subscribed)
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

	if container.Search == nil {
		t.Error("container.Search is nil")
	}
}

func TestContainerNew_ExposesConfigUsecase(t *testing.T) {
	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    &ucmocks.MockRuntime{},
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}

	container, err := New(repos, nil, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if container.Config == nil {
		t.Error("container.Config is nil")
	}
}
