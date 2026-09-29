package portable_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func runPortable(
	t *testing.T,
	req wizstep.Request,
	from string,
	to string,
	timeout string,
) error {
	t.Helper()

	h := portable.NewHandler(unpacktest.TestMaxBytes)
	s := domainstep.NewPortableStep("portable", from, to, timeout, true)

	return h.Execute(context.Background(), req, s)
}

func readRecord(
	t *testing.T,
	workDir string,
) domain.PortableRecord {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workDir, domain.PortableRecordFile))
	require.NoError(t, err)

	var record domain.PortableRecord
	require.NoError(t, json.Unmarshal(data, &record))

	return record
}
