package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func updateWire(t *testing.T, r models.UpdateResult) map[string]any {
	t.Helper()

	blob, err := json.Marshal(dto.UpdateResultDTOFrom(r))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(blob, &got))
	return got
}

// Nothing newer, nothing changed: every list is present and empty so a client
// can iterate without a null check, and there is no available ref.
func TestUpdateResultDTO_WireShape_Current(t *testing.T) {
	got := updateWire(t, models.UpdateResult{})

	assert.Equal(t, map[string]any{
		"added_deps":            []any{},
		"removed_from_manifest": []any{},
		"safe_to_uninstall":     []any{},
		"constrained_deps":      []any{},
	}, got)
}

func TestUpdateResultDTO_WireShape_Full(t *testing.T) {
	got := updateWire(t, models.UpdateResult{
		AddedDeps:           []domain.Namespace{"github.com/u/a@v1"},
		RemovedFromManifest: []domain.Namespace{"github.com/u/b@v1"},
		SafeToUninstall:     []domain.Namespace{"github.com/u/c@v1"},
		ConstrainedDeps: []models.ConstrainedDep{{
			Namespace:     "github.com/u/d",
			OldConstraint: "^1",
			NewConstraint: "^2",
		}},
		Available: &domain.Available{Ref: "v1.1.0", Commit: "c2"},
	})

	assert.Equal(t, []any{"github.com/u/a@v1"}, got["added_deps"])
	assert.Equal(t, []any{"github.com/u/b@v1"}, got["removed_from_manifest"])
	assert.Equal(t, []any{"github.com/u/c@v1"}, got["safe_to_uninstall"])
	assert.Equal(t, []any{map[string]any{
		"namespace":      "github.com/u/d",
		"old_constraint": "^1",
		"new_constraint": "^2",
	}}, got["constrained_deps"])
	assert.Equal(t, map[string]any{"ref": "v1.1.0", "commit": "c2"}, got["available"])
}
