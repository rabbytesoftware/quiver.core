package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const windowsHomeTemplate = `{{PROFILE}}\Documents\.quiver`

func TestExpandProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{"plain user", `C:\Users\alice`, `C:\Users\alice\Documents\.quiver`},
		{"computer name and user (bug case)", `C:\Users\playtester`, `C:\Users\playtester\Documents\.quiver`},
		{"domain account", `C:\Users\jdoe.CORP`, `C:\Users\jdoe.CORP\Documents\.quiver`},
		{"profile with spaces", `C:\Users\Mary Ann Smith`, `C:\Users\Mary Ann Smith\Documents\.quiver`},
		{"profile on another drive", `D:\Profiles\bob`, `D:\Profiles\bob\Documents\.quiver`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandProfile(windowsHomeTemplate, tt.profile)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "DESKTOP-61CQKU9")
		})
	}
}

func TestExpandProfile_DoesNotTouchTemplatesWithoutPlaceholder(t *testing.T) {
	assert.Equal(t, "~/.quiver", expandProfile("~/.quiver", `C:\Users\alice`))
}

func TestResolveHome_QuiverHomeOverride(t *testing.T) {
	t.Setenv(homeOverrideEnv, "override-dir")
	assert.Equal(t, "override-dir", resolveHome())
}

func TestResolveHome_UsesUserHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv(homeOverrideEnv, "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := resolveHome()
	assert.Contains(t, got, home)
	assert.NotContains(t, got, "{{")
}
