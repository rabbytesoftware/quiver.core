package pathenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func newWindowsFixture(
	t *testing.T,
) (*mocks.Sandbox, *windows) {
	t.Helper()
	sb := mocks.NewSandbox(t, "windows")
	return sb, NewWindows(sb.Host(), userpath.NewEditor(sb.UserPath)).(*windows)
}

func TestWindows_Setup(t *testing.T) {
	testCases := []struct {
		name           string
		current        func(bin string) string
		wantConfigured bool
		wantValue      func(bin string) string
		wantWrites     int
	}{
		{
			name:       "empty",
			current:    func(string) string { return "" },
			wantValue:  func(bin string) string { return bin },
			wantWrites: 1,
		},
		{
			name:       "others with trailing separator",
			current:    func(string) string { return `C:\a;` },
			wantValue:  func(bin string) string { return `C:\a;` + bin },
			wantWrites: 1,
		},
		{
			name:           "already present in another case",
			current:        func(bin string) string { return `C:\a;` + strings.ToUpper(bin) },
			wantConfigured: true,
			wantValue:      func(bin string) string { return `C:\a;` + strings.ToUpper(bin) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sb, m := newWindowsFixture(t)
			sb.UserPath.Value = tc.current(sb.Bin)

			before, err := m.Status(context.Background())
			require.NoError(t, err)
			after, err := m.Setup(context.Background())
			require.NoError(t, err)
			_, err = m.Setup(context.Background())
			require.NoError(t, err)

			assert.Equal(t, tc.wantConfigured, before.Configured)
			assert.True(t, after.Configured)
			assert.Equal(t, []string{`HKCU\Environment\Path`}, after.Files)
			assert.Equal(t, tc.wantValue(sb.Bin), sb.UserPath.Value)
			assert.Equal(t, tc.wantWrites, sb.UserPath.Writes)
			assert.Equal(t, tc.wantWrites, sb.UserPath.Broadcasts)
		})
	}
}

func TestWindows_Status_OnPath(t *testing.T) {
	sb, m := newWindowsFixture(t)
	sb.Env["PATH"] = `C:\Windows;` + strings.ToUpper(sb.Bin)

	got, err := m.Status(context.Background())

	require.NoError(t, err)
	assert.True(t, got.OnPath)
	assert.Equal(t, sb.Bin, got.BinDir)
}

func TestWindows_Errors(t *testing.T) {
	boom := errors.New("boom")

	sb, m := newWindowsFixture(t)
	sb.UserPath.ReadErr = boom
	_, err := m.Status(context.Background())
	assert.ErrorIs(t, err, boom)
	_, err = m.Setup(context.Background())
	assert.ErrorIs(t, err, boom)

	sb, m = newWindowsFixture(t)
	sb.UserPath.WriteErr = boom
	_, err = m.Setup(context.Background())
	assert.ErrorIs(t, err, boom)

	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, m = newWindowsFixture(t)
	m.host.HomeDir = file
	_, err = m.Status(context.Background())
	assert.Error(t, err)
	_, err = m.Setup(context.Background())
	assert.Error(t, err)
}
