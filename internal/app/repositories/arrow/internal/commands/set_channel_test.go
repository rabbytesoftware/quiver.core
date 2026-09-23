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

// TestSetChannel_EmitEvent_SetsChannelPreservesUnrelatedFields pins that a
// channel switch only touches the fields it has an opinion about
// (Channel, InstalledConstraint, Outdated, RecommendedRef — see
// TestSetChannel_EmitEvent_ClearsInstalledConstraint and
// TestSetChannel_EmitEvent_ClearsOutdatedAndRecommendedRef) and leaves
// everything else, like the arrow's own metadata, untouched.
func TestSetChannel_EmitEvent_SetsChannelPreservesUnrelatedFields(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "rc"}
	current := &domain.Arrow{
		Namespace: domain.Namespace("github.com/u/r@v1"),
		ArrowMeta: domain.ArrowMeta{Name: "r"},
	}

	next := cmd.EmitEvent(current)

	if next.Channel != "rc" {
		t.Errorf("Channel = %q, want %q", next.Channel, "rc")
	}
	if next.Name != "r" {
		t.Errorf("Name = %q, want %q (must be preserved)", next.Name, "r")
	}
}

// TestSetChannel_EmitEvent_ClearsInstalledConstraint guards the regression
// this test's own name describes: an arrow added via a glob install
// (resolveGlob) carries both InstalledConstraint AND a classified Channel.
// ResolveTrackedRef is constraint-first, so left in place, switching
// channel on such an arrow would have zero effect on drift-check
// resolution ever again — the constraint would always win. An explicit
// channel switch must supersede it.
func TestSetChannel_EmitEvent_ClearsInstalledConstraint(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "beta"}
	current := &domain.Arrow{
		Namespace:           domain.Namespace("github.com/u/r@v1"),
		InstalledConstraint: "^v1",
	}

	next := cmd.EmitEvent(current)

	if next.InstalledConstraint != "" {
		t.Errorf("InstalledConstraint = %q, want cleared", next.InstalledConstraint)
	}
	if next.Channel != "beta" {
		t.Errorf("Channel = %q, want %q", next.Channel, "beta")
	}
}

// TestSetChannel_EmitEvent_NoConstraintToClear_IsANoOp covers the self-
// registration caller (selfarrow.go's stampConfiguredChannel): it stamps a
// channel onto a row that never had a constraint at all, so clearing an
// already-empty field must not error or otherwise misbehave.
func TestSetChannel_EmitEvent_NoConstraintToClear_IsANoOp(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "stable"}
	current := &domain.Arrow{Namespace: domain.Namespace("github.com/u/r@v1")}

	next := cmd.EmitEvent(current)

	if next.InstalledConstraint != "" {
		t.Errorf("InstalledConstraint = %q, want empty", next.InstalledConstraint)
	}
}

// TestSetChannel_EmitEvent_ClearsOutdatedAndRecommendedRef guards the
// second half of the same regression: RecordVersionCheck is Outdated's and
// RecommendedRef's only other writer, and it writes nothing at all when a
// check errors (e.g. offline). Left stale after a channel switch, a
// previous channel's RecommendedRef could survive indefinitely, and since
// upgradeRef prefers RecommendedRef outright when set, a click on Update
// right after switching channels could upgrade onto the OLD channel's
// target instead of the new one.
func TestSetChannel_EmitEvent_ClearsOutdatedAndRecommendedRef(t *testing.T) {
	cmd := commands.SetChannel{Namespace: domain.Namespace("github.com/u/r@v1"), Channel: "beta"}
	current := &domain.Arrow{
		Namespace:      domain.Namespace("github.com/u/r@v1"),
		Outdated:       true,
		RecommendedRef: "v1.9.0",
		Channel:        "stable",
	}

	next := cmd.EmitEvent(current)

	if next.Outdated {
		t.Error("Outdated should be cleared to false")
	}
	if next.RecommendedRef != "" {
		t.Errorf("RecommendedRef = %q, want cleared", next.RecommendedRef)
	}
}
