package arrow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func portableNamed(
	name string,
) step.PortableStep {
	s := step.NewPortableStep("install", "${INSTALL_PATH}/.tool.download", "${INSTALL_PATH}/tool", "", true)
	s.Name = name
	return s
}

func TestPortableNameRule_Name(t *testing.T) {
	assert.Equal(t, "portable_name", PortableNameRule{}.Name())
}

func TestPortableNameRule_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		stepName  string
		wantField string
	}{
		{name: "no name keeps the source file name", stepName: ""},
		{name: "plain name", stepName: "tool"},
		{name: "windows executable name", stepName: "tool.exe"},
		{name: "path separator", stepName: "bin/tool", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "parent directory", stepName: "..", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "hidden name", stepName: ".tool", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "variable reference", stepName: "${NAME}", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "trailing dot", stepName: "tool.", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "trailing space", stepName: "tool ", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "device name", stepName: "CON", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "device name with extension", stepName: "nul.exe", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "lowercase serial port", stepName: "com3", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "printer port", stepName: "LPT9.bin", wantField: "targets[linux/amd64].lifecycle.install[1].name"},
		{name: "device-like but allowed", stepName: "console"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &domain.Arrow{Targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {Lifecycle: domain.TargetLifecycle{Install: step.StepList{
					step.NewRunStep("prepare", "echo ok", false, "", true),
					portableNamed(tc.stepName),
				}}},
			}}

			errs := PortableNameRule{}.Validate(m)

			if tc.wantField == "" {
				assert.Nil(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Equal(t, "invalid_name", errs[0].Rule)
			assert.Equal(t, tc.wantField, errs[0].Field)
		})
	}
}

func TestPortableNameRule_ChecksMethodSteps(t *testing.T) {
	m := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {Methods: map[string]domain.Method{
			"repair": {Steps: step.StepList{portableNamed("a/b")}},
		}},
	}}

	errs := PortableNameRule{}.Validate(m)

	require.Len(t, errs, 1)
	assert.Equal(t, "targets[linux/amd64].methods[repair].steps[0].name", errs[0].Field)
}

func TestPortableNameRule_ReportsLifecycleListsInOrder(t *testing.T) {
	m := &domain.Arrow{Targets: map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {Lifecycle: domain.TargetLifecycle{
			Install:      step.StepList{portableNamed("a/b")},
			Update:       step.StepList{portableNamed("CON")},
			Preinstalled: step.StepList{portableNamed("x.")},
		}},
	}}

	errs := PortableNameRule{}.Validate(m)

	require.Len(t, errs, 3)
	assert.Equal(t, "targets[linux/amd64].lifecycle.install[0].name", errs[0].Field)
	assert.Equal(t, "targets[linux/amd64].lifecycle.update[0].name", errs[1].Field)
	assert.Equal(t, "targets[linux/amd64].lifecycle.preinstalled[0].name", errs[2].Field)
}
