package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestArrowEventDTOFrom_Upserted(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace:     "github.com/user/repo@v1.0.0",
			ArrowMeta:     domain.ArrowMeta{Name: "repo", Description: "desc", Tags: []string{"a"}},
			UserInstalled: true,
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "upserted", m["event"])
	assert.Equal(t, "github.com/user/repo@v1.0.0", m["namespace"])
	assert.Equal(t, "repo", m["name"])
	assert.NotContains(t, m, "version", "the ref in the namespace is the version; the event carries no second copy")
	assert.Equal(t, "desc", m["description"])
	assert.Equal(t, true, m["user_installed"])
}

func TestArrowEventDTOFrom_Removed(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind:  hub.CatalogRemoved,
		Arrow: domain.Arrow{Namespace: "github.com/user/repo@v1.0.0"},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "removed", m["event"])
	assert.Equal(t, "github.com/user/repo@v1.0.0", m["namespace"])
}

func TestArrowDTOFrom(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo@v1.0.0",
		ArrowMeta: domain.ArrowMeta{Name: "Test"},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "github.com/user/repo@v1.0.0", d.Namespace, "the namespace carries the ref, which is the arrow's version")
	assert.Equal(t, "Test", d.Name)

	data, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"namespace"`)

	// user_installed must be propagated so arrowWatcher can filter correctly.
	aInstalled := domain.Arrow{
		Namespace:     "github.com/user/repo",
		ArrowMeta:     domain.ArrowMeta{Name: "Test"},
		UserInstalled: true,
	}
	dInstalled := dto.ArrowDTOFrom(aInstalled)
	assert.True(t, dInstalled.UserInstalled, "UserInstalled must be propagated to ArrowDTO")

	dataInstalled, err := json.Marshal(dInstalled)
	require.NoError(t, err)
	assert.Contains(t, string(dataInstalled), `"user_installed":true`)
}

func TestArrowDTOFrom_Origin(t *testing.T) {
	generator := &domain.ArrowGenerator{Name: "generator/1", Confidence: "high"}
	testCases := []struct {
		name          string
		generator     *domain.ArrowGenerator
		wantOrigin    string
		wantInference *dto.InferenceDTO
	}{
		{name: "declared", wantOrigin: "declared"},
		{
			name:          "inferred",
			generator:     generator,
			wantOrigin:    "inferred",
			wantInference: dto.InferenceDTOFrom(generator),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := dto.ArrowDTOFrom(domain.Arrow{
				Namespace: "github.com/user/repo",
				ArrowMeta: domain.ArrowMeta{Name: "Test", Generator: tc.generator},
			})
			assert.Equal(t, tc.wantOrigin, d.Origin)
			assert.Equal(t, tc.wantInference, d.Inference)
		})
	}
}

func TestArrowDTOFrom_MediaMapped(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo",
		ArrowMeta: domain.ArrowMeta{
			Name:  "Test",
			Media: domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
		},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "https://example.com/icon.png", d.Media.Icon)
	assert.Equal(t, "https://example.com/banner.png", d.Media.Banner)
}

func TestArrowDTOFrom_LastUsedAtMapped(t *testing.T) {
	lastUsed := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	a := domain.Arrow{
		Namespace:  "github.com/user/repo@v1.0.0",
		ArrowMeta:  domain.ArrowMeta{Name: "Test"},
		LastUsedAt: lastUsed,
	}
	d := dto.ArrowDTOFrom(a)
	assert.Equal(t, "2026-08-01T09:30:00Z", d.LastUsedAt)
}

func TestArrowDTOFrom_NeverUsed_LastUsedAtOmitted(t *testing.T) {
	a := domain.Arrow{
		Namespace: "github.com/user/repo@v1.0.0",
		ArrowMeta: domain.ArrowMeta{Name: "Test"},
	}
	d := dto.ArrowDTOFrom(a)
	assert.Empty(t, d.LastUsedAt)

	data, err := json.Marshal(d)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	assert.NotContains(t, m, "last_used_at")
}

func TestArrowEventDTOFrom_UpsertedIncludesLastUsedAt(t *testing.T) {
	lastUsed := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace:  "github.com/user/repo@v1.0.0",
			ArrowMeta:  domain.ArrowMeta{Name: "repo"},
			LastUsedAt: lastUsed,
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "2026-08-01T09:30:00Z", m["last_used_at"])
}

func TestArrowEventDTOFrom_UpsertedIncludesMedia(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace: "github.com/user/repo@v1.0.0",
			ArrowMeta: domain.ArrowMeta{
				Name:  "repo",
				Media: domain.ArrowMedia{Icon: "https://example.com/icon.png", Banner: "https://example.com/banner.png"},
			},
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	media, ok := m["media"].(map[string]any)
	require.True(t, ok, "media key must be present")
	assert.Equal(t, "https://example.com/icon.png", media["icon"])
	assert.Equal(t, "https://example.com/banner.png", media["banner"])
}

// The catalog stream keeps its shape: the selector bookkeeping lives on the
// detail endpoint and must not ride along in every upsert.
func TestArrowEventDTOFrom_WireShape_Unchanged(t *testing.T) {
	evt := hub.ArrowEvent{
		Kind: hub.CatalogUpserted,
		Arrow: domain.Arrow{
			Namespace:    "github.com/user/repo@stable",
			ArrowMeta:    domain.ArrowMeta{Name: "repo"},
			SelectorKind: domain.SelectorChannel,
			Resolved:     domain.Resolved{Ref: "v1.0.0", Commit: "c1"},
			Available:    &domain.Available{Ref: "v1.1.0", Commit: "c2"},
		},
	}
	data, err := json.Marshal(dto.ArrowEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.ElementsMatch(t,
		[]string{"event", "namespace", "name", "description", "tags", "media", "user_installed", "origin"},
		mapKeys(m))
}

// A pin is the domain's empty kind; the stream must not surface it as an empty
// selector_kind, nor any other versioning field.
func TestArrowEventDTOFrom_WireShape_PinRowCarriesNoSelector(t *testing.T) {
	for _, kind := range []hub.CatalogEventKind{hub.CatalogUpserted, hub.CatalogRemoved} {
		data, err := json.Marshal(dto.ArrowEventDTOFrom(hub.ArrowEvent{
			Kind:  kind,
			Arrow: domain.Arrow{Namespace: "github.com/user/repo@v1.0.0", SelectorKind: domain.SelectorPin},
		}))
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		for _, key := range []string{
			"selector_kind", "resolved", "resolved_ref", "installed_commit", "available",
			"channel", "installed_constraint", "recommended_ref", "pinned_ref",
			"ref_is_branch", "ref_commit_sha", "outdated", "installed_ref",
		} {
			assert.NotContains(t, m, key)
		}
	}
}
