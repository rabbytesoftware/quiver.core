package usecases

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
)

func TestContainerNew_ExposesConfigUsecase(t *testing.T) {
	repos := &repositories.Container{
		Arrow:      &ucmocks.MockArrow{},
		Runtime:    &ucmocks.MockRuntime{},
		Collection: &ucmocks.MockCollection{},
		Graph:      &ucmocks.MockGraph{},
	}

	container, err := New(repos, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
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

	if container.System == nil {
		t.Error("container.System is nil")
	}

	if container.Config == nil {
		t.Error("container.Config is nil")
	}
}
