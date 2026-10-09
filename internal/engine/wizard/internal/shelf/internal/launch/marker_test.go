package launch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarker_RecordRecordedClear(t *testing.T) {
	dir := t.TempDir()

	got, err := Recorded(dir)
	require.NoError(t, err)
	assert.Empty(t, got, "a workdir without a marker has no target")

	require.NoError(t, Record(dir, "/apps/Tool.app"))
	got, err = Recorded(dir)
	require.NoError(t, err)
	assert.Equal(t, "/apps/Tool.app", got)

	require.NoError(t, Clear(dir))
	require.NoError(t, Clear(dir), "clearing twice is not an error")
	got, err = Recorded(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}
