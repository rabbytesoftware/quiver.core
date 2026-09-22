package commands_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestSetChannel_AggregateID_ReturnsNamespaceString(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1")}
	if got, want := cmd.AggregateID(), "github.com/u/r@v1"; got != want {
		t.Errorf("AggregateID() = %q, want %q", got, want)
	}
}

func TestSetChannel_Validate_NilCurrent_ReturnsValidationError(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "rc"}
	if err := cmd.Validate(nil); err == nil {
		t.Fatal("expected error for nil current")
	}
}

func TestSetChannel_Validate_ExistingCurrent_NoError(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "rc"}
	current := &domain.Arrow{Namespace: domain.Namespace("github.com/u/r@v1")}
	if err := cmd.Validate(current); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetChannel_EmitEvent_SetsChannelPreservesRest(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "rc"}
	current := &domain.Arrow{
		Namespace: domain.Namespace("github.com/u/r@v1"),
		ArrowMeta: domain.ArrowMeta{Name: "r"},
		Outdated:  true,
	}

	next := cmd.EmitEvent(current)

	if next.Channel != "rc" {
		t.Errorf("Channel = %q, want %q", next.Channel, "rc")
	}
	if next.Name != "r" {
		t.Errorf("Name = %q, want %q (must be preserved)", next.Name, "r")
	}
	if !next.Outdated {
		t.Error("Outdated should be preserved as true")
	}
}
