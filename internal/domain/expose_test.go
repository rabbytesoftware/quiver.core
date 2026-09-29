package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExpose_IsEmpty(t *testing.T) {
	testCases := []struct {
		name   string
		expose Expose
		want   bool
	}{
		{
			name:   "zero value",
			expose: Expose{},
			want:   true,
		},
		{
			name:   "empty slices",
			expose: Expose{CLI: []ExposeEntry{}, Desktop: []ExposeEntry{}},
			want:   true,
		},
		{
			name:   "has cli entry",
			expose: Expose{CLI: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}}},
			want:   false,
		},
		{
			name:   "has desktop entry",
			expose: Expose{Desktop: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}}},
			want:   false,
		},
		{
			name: "has both",
			expose: Expose{
				CLI:     []ExposeEntry{{Name: "mytool", Path: ExposeAuto}},
				Desktop: []ExposeEntry{{Name: "mytool", Path: ExposeAuto}},
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.expose.IsEmpty())
		})
	}
}

func TestExposeAuto_Value(t *testing.T) {
	assert.Equal(t, "auto", ExposeAuto)
}

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

func TestExposeKind_Values(t *testing.T) {
	testCases := []struct {
		name string
		kind ExposeKind
		want string
	}{
		{
			name: "cli",
			kind: ExposeKindCLI,
			want: "cli",
		},
		{
			name: "desktop",
			kind: ExposeKindDesktop,
			want: "desktop",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(tc.kind))
		})
	}
}
