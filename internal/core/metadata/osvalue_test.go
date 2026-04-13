package metadata

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestOsValue_UnmarshalYAML_Scalar(t *testing.T) {
	var v OsValue[string]
	require.NoError(t, yaml.Unmarshal([]byte(`"~/.quiver"`), &v))
	assert.Equal(t, "~/.quiver", v.Default)
	assert.Empty(t, v.OS)
}

func TestOsValue_UnmarshalYAML_Map(t *testing.T) {
	input := `
default: "~/.quiver"
windows: 'C:\Users\{{USER}}\Documents\.quiver'
`
	var v OsValue[string]
	require.NoError(t, yaml.Unmarshal([]byte(input), &v))
	assert.Equal(t, "~/.quiver", v.Default)
	assert.Equal(t, `C:\Users\{{USER}}\Documents\.quiver`, v.OS["windows"])
}

func TestOsValue_UnmarshalYAML_InvalidNode(t *testing.T) {
	var v OsValue[string]
	err := yaml.Unmarshal([]byte("- item1\n- item2"), &v)
	assert.Error(t, err)
}

func TestOsValue_Resolve_UsesOSOverride(t *testing.T) {
	v := OsValue[string]{
		Default: "default-val",
		OS:      map[string]string{runtime.GOOS: "os-val"},
	}
	assert.Equal(t, "os-val", v.Resolve())
}

func TestOsValue_Resolve_FallsBackToDefault(t *testing.T) {
	v := OsValue[string]{
		Default: "default-val",
		OS:      map[string]string{"other-os": "other-val"},
	}
	assert.Equal(t, "default-val", v.Resolve())
}

func TestOsValue_Resolve_EmptyOS(t *testing.T) {
	v := OsValue[string]{Default: "only-default"}
	assert.Equal(t, "only-default", v.Resolve())
}
