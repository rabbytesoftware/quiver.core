package tui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func TestCheckPayload_AcceptsOrdinaryPayloads(t *testing.T) {
	testCases := []struct {
		name    string
		payload any
	}{
		{"struct", arrow{Namespace: "github.com/u/r", State: "ready"}},
		{"slice of structs", []arrow{{Namespace: "a"}, {Namespace: "b"}}},
		{"map", map[string]string{"name": "repo"}},
		{"nil", nil},
		{"scalar", 42},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, tui.CheckPayload(tc.payload))
		})
	}
}

func TestCheckPayload_RejectsUnencodableTypes(t *testing.T) {
	testCases := []struct {
		name    string
		payload any
	}{
		{"channel", map[string]any{"ch": make(chan int)}},
		{"function", map[string]any{"fn": func() {}}},
		{"complex", map[string]any{"c": complex(1, 2)}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tui.CheckPayload(tc.payload)

			require.Error(t, err, "a payload that would crash the CLI must fail the test")
			assert.Contains(t, err.Error(), "not json-encodable",
				"json rejects these before yaml is reached")
		})
	}
}

// panicker only fails during yaml encoding, which is the class of payload that
// would otherwise reach a user as a crash rather than an error.
type panicker struct {
	Name string `json:"name" yaml:"name"`
}

func (panicker) MarshalYAML() (any, error) { panic("cannot represent this value") }

func TestCheckPayload_RecoversAPanicFromTheYAMLEncoder(t *testing.T) {
	err := tui.CheckPayload(panicker{Name: "repo"})

	require.Error(t, err, "a yaml panic must become an error, not escape")
	assert.Contains(t, err.Error(), "not encodable")
	assert.Contains(t, err.Error(), "cannot represent this value")
}

// A payload whose fields carry a json tag but no yaml tag serializes under two
// different key names, which is why every DTO must carry both.
func TestCheckPayload_DoesNotCatchTagDivergence(t *testing.T) {
	type mismatched struct {
		InstalledConstraint string `json:"installed_constraint"`
	}

	assert.NoError(t, tui.CheckPayload(mismatched{InstalledConstraint: "^1.2"}),
		"CheckPayload proves encodability, not key parity")
}
