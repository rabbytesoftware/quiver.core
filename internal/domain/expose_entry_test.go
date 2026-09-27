package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExposeEntry_JSONRoundTrip(t *testing.T) {
	testCases := []struct {
		name  string
		entry ExposeEntry
	}{
		{
			name: "full entry",
			entry: ExposeEntry{
				Name:       "mytool",
				Path:       "${INSTALL_PATH}/bin/mytool",
				Icon:       "${INSTALL_PATH}/icon.png",
				Categories: []string{"Utility", "Development"},
			},
		},
		{
			name: "minimal entry",
			entry: ExposeEntry{
				Name: "mytool",
				Path: "auto",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.entry)
			require.NoError(t, err)

			var got ExposeEntry
			require.NoError(t, json.Unmarshal(data, &got))
			require.Equal(t, tc.entry, got)
		})
	}
}

func TestExposeEntry_YAMLRoundTrip(t *testing.T) {
	entry := ExposeEntry{
		Name:       "mytool",
		Path:       "${WORKDIR}/mytool",
		Categories: []string{"Game"},
	}

	data, err := yaml.Marshal(entry)
	require.NoError(t, err)

	var got ExposeEntry
	require.NoError(t, yaml.Unmarshal(data, &got))
	require.Equal(t, entry, got)
}
