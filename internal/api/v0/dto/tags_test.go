package dto_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// keysOf marshals v with enc, decodes the result, and returns its top-level
// keys.
func keysOf(t *testing.T, v any, marshal func(any) ([]byte, error)) []string {
	t.Helper()

	raw, err := marshal(v)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &decoded), "yaml decodes json too")

	keys := make([]string, 0, len(decoded))
	for k := range decoded {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// A DTO carrying a json tag but no yaml tag serializes under two different key
// names, because yaml.v3 falls back to the lowercased Go field name. That made
// `-o yaml` disagree with the documented API: InstalledConstraint emitted
// "installed_constraint" as json and "installedconstraint" as yaml.
func TestDTO_JSONAndYAMLKeysAgree(t *testing.T) {
	testCases := []struct {
		name  string
		value any
	}{
		{"arrow detail", dto.ArrowDetailDTO{
			Namespace:           "github.com/u/r",
			InstalledAt:         "2026-08-16T00:00:00Z",
			InstalledConstraint: "^1.2",
			UserInstalled:       true,
		}},
		{"arrow list item", dto.ArrowListItemDTO{Namespace: "github.com/u/r", Name: "repo"}},
		{"installed version", dto.InstalledVersionItemDTO{
			Ref:         "v1.2.0",
			State:       "ready",
			InstalledAt: "2026-08-16T00:00:00Z",
			Constraint:  "^1.2",
		}},
		{"collection arrow", dto.CollectionArrowDTO{Namespace: "github.com/u/c", Resolved: true}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsonKeys := keysOf(t, tc.value, func(v any) ([]byte, error) { return json.Marshal(v) })
			yamlKeys := keysOf(t, tc.value, func(v any) ([]byte, error) { return yaml.Marshal(v) })

			assert.Equal(t, jsonKeys, yamlKeys,
				"every DTO field needs both a json and a yaml tag")
		})
	}
}
