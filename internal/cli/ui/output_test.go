package ui_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

func TestResolveFormat_ExplicitWins(t *testing.T) {
	f, err := ui.ResolveFormat("yaml", true)
	require.NoError(t, err)
	assert.Equal(t, "yaml", f)
}

func TestResolveFormat_DefaultTableOnTTY(t *testing.T) {
	f, err := ui.ResolveFormat("", true)
	require.NoError(t, err)
	assert.Equal(t, "table", f)
}

func TestResolveFormat_DefaultJSONInPipe(t *testing.T) {
	f, err := ui.ResolveFormat("", false)
	require.NoError(t, err)
	assert.Equal(t, "json", f)
}

func TestResolveFormat_UnknownErrors(t *testing.T) {
	_, err := ui.ResolveFormat("xml", true)
	assert.Error(t, err)
}

func TestWriteJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, ui.WriteJSON(&buf, map[string]string{"a": "b"}))
	assert.Contains(t, buf.String(), "\"a\": \"b\"")
}

func TestWriteYAML_Marshals(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, ui.WriteYAML(&buf, map[string]string{"a": "b"}))
	assert.Contains(t, buf.String(), "a: b")
}

func TestWriteJSON_UnmarshalableErrors(t *testing.T) {
	var buf bytes.Buffer
	assert.Error(t, ui.WriteJSON(&buf, make(chan int)))
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, assert.AnError }

func TestWriteYAML_WriterFailureErrors(t *testing.T) {
	assert.Error(t, ui.WriteYAML(failingWriter{}, map[string]string{"a": "b"}))
}
