package shelf

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathStatus_JSON(t *testing.T) {
	data, err := json.Marshal(PathStatus{BinDir: "/b", OnPath: true, Configured: false, Files: []string{"/rc"}})
	require.NoError(t, err)

	assert.JSONEq(t, `{"bin_dir":"/b","on_path":true,"configured":false,"files":["/rc"]}`, string(data))
}
