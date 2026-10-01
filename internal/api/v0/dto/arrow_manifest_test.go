package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

func TestArrowManifestDTOFrom_Nil_ReturnsNil(t *testing.T) {
	result := dto.ArrowManifestDTOFrom(nil)
	assert.Nil(t, result)
}

func TestArrowManifestDTOFrom_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Test Arrow", License: "MIT"},
		Netbridge: []netbridge.PortDef{{Name: "http", Default: 8080}},
		Readme:    "# Docs",
	}
	input := &models.ArrowManifestDTO{
		Namespace:   ns,
		Name:        "Test Arrow",
		Description: "A test arrow",
		Tags:        []string{"tag1", "tag2"},
		Variables:   []domain.Variable{{Name: "VAR", Default: "val"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {},
		},
		Manifest: arrow,
	}

	result := dto.ArrowManifestDTOFrom(input)
	require.NotNil(t, result)
	assert.Equal(t, string(ns), result.Namespace)
	assert.Equal(t, "Test Arrow", result.Name)
	assert.Equal(t, "A test arrow", result.Description)
	assert.Equal(t, "v1.0.0", domain.Namespace(result.Namespace).Ref(), "the namespace is the only thing that names the version")
	assert.Equal(t, []string{"tag1", "tag2"}, result.Tags)
	assert.Len(t, result.Variables, 1)
	assert.Len(t, result.Targets, 1)
	require.NotNil(t, result.Manifest)
	assert.Equal(t, arrow.ArrowMeta, result.Manifest.Metadata)
	assert.Equal(t, arrow.Netbridge, result.Manifest.Netbridge)
	assert.Equal(t, "# Docs", result.Manifest.Readme)
}

func TestArrowManifestDTOFrom_NoManifest_OmitsContent(t *testing.T) {
	result := dto.ArrowManifestDTOFrom(&models.ArrowManifestDTO{Namespace: "github.com/user/repo@v1"})

	require.NotNil(t, result)
	assert.Nil(t, result.Manifest)
}

// The manifest response is what the arrow's author wrote. Quiver's own
// bookkeeping about the installed row belongs to the detail endpoint.
func TestArrowManifestDTO_WireShape_ContentWithoutBookkeeping(t *testing.T) {
	arrow := &domain.Arrow{
		Namespace:     "github.com/user/repo@stable",
		ArrowMeta:     domain.ArrowMeta{Name: "App", Description: "An app", Tags: []string{"web"}},
		Variables:     []domain.Variable{{Name: "VAR"}},
		Netbridge:     []netbridge.PortDef{{Name: "http", Default: 8080}},
		Targets:       map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		Readme:        "# App",
		InstalledAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UserInstalled: true,
		LastUsedAt:    time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		SelectorKind:  domain.SelectorChannel,
		Resolved:      domain.Resolved{Ref: "v1.0.0", Commit: "c1", Fingerprint: "fp"},
		Available:     &domain.Available{Ref: "v1.1.0", Commit: "c2"},
	}

	blob, err := json.Marshal(dto.ArrowManifestDTOFrom(&models.ArrowManifestDTO{
		Namespace: arrow.Namespace,
		Name:      arrow.Name,
		Manifest:  arrow,
	}))
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(blob, &got))

	manifest, ok := got["manifest"].(map[string]any)
	require.True(t, ok, "manifest must be an object: %s", blob)
	assert.ElementsMatch(t,
		[]string{"metadata", "variables", "netbridge", "targets", "readme"},
		mapKeys(manifest))

	metadata, ok := manifest["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "App", metadata["name"])
	assert.Equal(t, "An app", metadata["description"])
	assert.Equal(t, "# App", manifest["readme"])

	bookkeeping := []string{
		"selector_kind", "resolved", "available", "installed_at",
		"user_installed", "last_used_at", "resolved_ref", "installed_commit",
	}
	for _, key := range bookkeeping {
		assert.NotContains(t, got, key)
		assert.NotContains(t, manifest, key)
		assert.NotContains(t, metadata, key)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
