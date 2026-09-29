package install_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"

	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/install"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func runPortable(
	t *testing.T,
	req wizstep.Request,
	from string,
	to string,
	timeout string,
) error {
	t.Helper()

	return runPortableWith(t, mocks.TestMaxBytes, req, from, to, timeout, "")
}

func runPortableNamed(
	t *testing.T,
	req wizstep.Request,
	from string,
	to string,
	name string,
) error {
	t.Helper()

	return runPortableWith(t, mocks.TestMaxBytes, req, from, to, "", name)
}

func runPortableWith(
	t *testing.T,
	maxBytes int64,
	req wizstep.Request,
	from string,
	to string,
	timeout string,
	name string,
) error {
	t.Helper()

	ctx, cancel, err := wizstep.WithTimeout(context.Background(), timeout)
	if err != nil {
		return err
	}
	defer cancel()

	from = req.ResolvePath(from)
	to = req.ResolvePath(to)
	installer := install.New(maxBytes)

	apps, output, err := installer.Place(ctx, req.OSArch, req.WorkDir, from, to, name)
	if err != nil {
		return err
	}
	if err := installer.Record(ctx, req.NSKey, req.WorkDir, apps); err != nil {
		return err
	}

	return installer.RemoveSource(req.WorkDir, from, output)
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
