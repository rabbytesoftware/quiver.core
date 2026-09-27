package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPortableApp_JSONRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		app  PortableApp
	}{
		{
			name: "full app",
			app: PortableApp{
				Name:  "Bruno",
				Entry: "bruno_4.2.0_arm64_linux/.quiver-run",
				Icon:  "bruno_4.2.0_arm64_linux/usr/share/icons/hicolor/1024x1024/apps/bruno.png",
			},
		},
		{
			name: "no icon",
			app: PortableApp{
				Name:  "Firefox",
				Entry: "Firefox.app",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.app)
			require.NoError(t, err)

			var got PortableApp
			require.NoError(t, json.Unmarshal(data, &got))
			require.Equal(t, tc.app, got)
		})
	}
}

func TestPortableApp_YAMLRoundTrip(t *testing.T) {
	app := PortableApp{
		Name:  "Bruno",
		Entry: "bruno/.quiver-run",
		Icon:  "bruno/icon.png",
	}

	data, err := yaml.Marshal(app)
	require.NoError(t, err)

	var got PortableApp
	require.NoError(t, yaml.Unmarshal(data, &got))
	require.Equal(t, app, got)
}

func TestPortableApp_JSONOmitsEmptyIcon(t *testing.T) {
	app := PortableApp{Name: "Firefox", Entry: "Firefox.app"}

	data, err := json.Marshal(app)
	require.NoError(t, err)

	require.NotContains(t, string(data), "icon")
}
