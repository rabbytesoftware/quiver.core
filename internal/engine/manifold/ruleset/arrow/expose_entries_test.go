package arrow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestExposeEntriesRule_Name(t *testing.T) {
	assert.Equal(t, "expose_entries", ExposeEntriesRule{}.Name())
}

func TestExposeEntriesRule_Validate(t *testing.T) {
	cli := func(entries ...domain.ExposeEntry) domain.Expose { return domain.Expose{CLI: entries} }
	desktop := func(entries ...domain.ExposeEntry) domain.Expose { return domain.Expose{Desktop: entries} }
	auto := func(name string) domain.ExposeEntry { return domain.ExposeEntry{Name: name, Path: domain.ExposeAuto} }
	icon := func(value string) domain.ExposeEntry {
		return domain.ExposeEntry{Name: "mytool", Path: domain.ExposeAuto, Icon: value}
	}

	testCases := []struct {
		name     string
		os       domain.OS
		expose   domain.Expose
		wantRule string
	}{
		{name: "empty expose"},
		{name: "cli entry under install path", expose: cli(domain.ExposeEntry{Name: "mytool", Path: "${INSTALL_PATH}/bin/mytool"})},
		{name: "cli entry under workdir", expose: cli(domain.ExposeEntry{Name: "mytool", Path: "${WORKDIR}/mytool"})},
		{name: "auto path", expose: cli(auto("mytool"))},
		{name: "unanchored path", expose: cli(domain.ExposeEntry{Name: "mytool", Path: "/usr/local/bin/mytool"}), wantRule: "invalid_path"},
		{name: "path traversal", expose: cli(domain.ExposeEntry{Name: "mytool", Path: "${INSTALL_PATH}/../escape"}), wantRule: "path_traversal"},
		{name: "empty name", expose: cli(auto("")), wantRule: "invalid_name"},
		{name: "name with invalid characters", expose: cli(auto("my tool!")), wantRule: "invalid_name"},
		{name: "duplicate names within a kind", expose: cli(auto("mytool"), auto("mytool")), wantRule: "duplicate_name"},
		{name: "same name across kinds", expose: domain.Expose{CLI: []domain.ExposeEntry{auto("mytool")}, Desktop: []domain.ExposeEntry{auto("mytool")}}},
		{name: "darwin desktop auto path", os: domain.OSDarwinARM64, expose: desktop(auto("MyApp"))},
		{name: "darwin desktop .app path", os: domain.OSDarwinARM64, expose: desktop(domain.ExposeEntry{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp.app"})},
		{name: "darwin desktop path without .app", os: domain.OSDarwinARM64, expose: desktop(domain.ExposeEntry{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp"}), wantRule: "invalid_desktop_path"},
		{name: "linux desktop path without .app", expose: desktop(domain.ExposeEntry{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp"})},
		{name: "http icon", expose: cli(icon("http://example.com/icon.png"))},
		{name: "https icon", expose: cli(icon("https://example.com/icon.png"))},
		{name: "install-path icon", expose: cli(icon("${INSTALL_PATH}/icon.png"))},
		{name: "workdir icon", expose: cli(icon("${WORKDIR}/icon.png"))},
		{name: "auto icon", expose: cli(icon(domain.ExposeAuto)), wantRule: "invalid_expose_icon"},
		{name: "unanchored icon", expose: cli(icon("/etc/icon.png")), wantRule: "invalid_expose_icon"},
		{name: "traversal icon", expose: cli(icon("${INSTALL_PATH}/../etc/icon.png")), wantRule: "invalid_expose_icon"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			os := tc.os
			if os == "" {
				os = domain.OSLinuxAMD64
			}

			errs := ExposeEntriesRule{}.Validate(&domain.Arrow{Targets: map[domain.OS]domain.Target{os: {Expose: tc.expose}}})

			if tc.wantRule == "" {
				assert.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			assert.Equal(t, tc.wantRule, errs[0].Rule)
		})
	}
}
