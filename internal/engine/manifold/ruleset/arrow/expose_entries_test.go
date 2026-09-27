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
	testCases := []struct {
		name     string
		targets  map[domain.OS]domain.Target
		wantRule string
		wantErr  bool
	}{
		{
			name:    "no targets",
			targets: map[domain.OS]domain.Target{},
		},
		{
			name: "empty expose",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {},
			},
		},
		{
			name: "valid cli entry with install path",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: "${INSTALL_PATH}/bin/mytool"},
						},
					},
				},
			},
		},
		{
			name: "valid cli entry with workdir",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: "${WORKDIR}/mytool"},
						},
					},
				},
			},
		},
		{
			name: "valid auto path",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto},
						},
					},
				},
			},
		},
		{
			name: "invalid path prefix",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: "/usr/local/bin/mytool"},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_path",
		},
		{
			name: "path traversal",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: "${INSTALL_PATH}/../escape"},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "path_traversal",
		},
		{
			name: "invalid name empty",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "", Path: domain.ExposeAuto},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_name",
		},
		{
			name: "invalid name characters",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "my tool!", Path: domain.ExposeAuto},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_name",
		},
		{
			name: "duplicate names within cli",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto},
							{Name: "mytool", Path: domain.ExposeAuto},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "duplicate_name",
		},
		{
			name: "same name allowed across kinds",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI:     []domain.ExposeEntry{{Name: "mytool", Path: domain.ExposeAuto}},
						Desktop: []domain.ExposeEntry{{Name: "mytool", Path: domain.ExposeAuto}},
					},
				},
			},
		},
		{
			name: "darwin desktop entry auto path allowed",
			targets: map[domain.OS]domain.Target{
				domain.OSDarwinARM64: {
					Expose: domain.Expose{
						Desktop: []domain.ExposeEntry{
							{Name: "MyApp", Path: domain.ExposeAuto},
						},
					},
				},
			},
		},
		{
			name: "darwin desktop entry ends with .app",
			targets: map[domain.OS]domain.Target{
				domain.OSDarwinARM64: {
					Expose: domain.Expose{
						Desktop: []domain.ExposeEntry{
							{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp.app"},
						},
					},
				},
			},
		},
		{
			name: "darwin desktop entry missing .app suffix",
			targets: map[domain.OS]domain.Target{
				domain.OSDarwinARM64: {
					Expose: domain.Expose{
						Desktop: []domain.ExposeEntry{
							{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp"},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_desktop_path",
		},
		{
			name: "linux desktop entry without .app suffix is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						Desktop: []domain.ExposeEntry{
							{Name: "MyApp", Path: "${INSTALL_PATH}/MyApp"},
						},
					},
				},
			},
		},
		{
			name: "empty icon is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: ""},
						},
					},
				},
			},
		},
		{
			name: "http icon is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "http://example.com/icon.png"},
						},
					},
				},
			},
		},
		{
			name: "https icon is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "https://example.com/icon.png"},
						},
					},
				},
			},
		},
		{
			name: "install-path-anchored icon is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "${INSTALL_PATH}/icon.png"},
						},
					},
				},
			},
		},
		{
			name: "workdir-anchored icon is valid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "${WORKDIR}/icon.png"},
						},
					},
				},
			},
		},
		{
			name: "auto icon is invalid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: domain.ExposeAuto},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_expose_icon",
		},
		{
			name: "unanchored icon is invalid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "/etc/icon.png"},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_expose_icon",
		},
		{
			name: "traversal icon is invalid",
			targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Expose: domain.Expose{
						CLI: []domain.ExposeEntry{
							{Name: "mytool", Path: domain.ExposeAuto, Icon: "${INSTALL_PATH}/../etc/icon.png"},
						},
					},
				},
			},
			wantErr:  true,
			wantRule: "invalid_expose_icon",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := ExposeEntriesRule{}
			m := &domain.Arrow{Targets: tc.targets}
			errs := rule.Validate(m)

			if !tc.wantErr {
				assert.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			assert.Equal(t, tc.wantRule, errs[0].Rule)
		})
	}
}
