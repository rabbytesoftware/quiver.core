//go:build windows

package userpath

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

// scratch points a registry user path at a throwaway HKCU\Software key, so the
// test never reads or writes the real HKCU\Environment.
func scratch(
	t *testing.T,
) (*registryUserPath, registry.Key) {
	t.Helper()
	suffix := make([]byte, 8)
	_, err := rand.Read(suffix)
	require.NoError(t, err)
	subkey := `Software\QuiverTest-` + hex.EncodeToString(suffix)

	k, _, err := registry.CreateKey(registry.CURRENT_USER, subkey, registry.ALL_ACCESS)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = k.Close()
		_ = registry.DeleteKey(registry.CURRENT_USER, subkey)
	})
	return &registryUserPath{root: registry.CURRENT_USER, subkey: subkey, area: "QuiverTest"}, k
}

func TestRegistry_New_TargetsTheUserEnvironment(t *testing.T) {
	assert.Equal(t, `HKCU\Environment\Path`, New().Location())
}

func TestRegistry_ReadWrite_NewValueIsExpandable(t *testing.T) {
	up, k := scratch(t)

	empty, err := up.Read()
	require.NoError(t, err)
	require.NoError(t, up.Write(`C:\a;%USERPROFILE%\b`))
	got, err := up.Read()
	require.NoError(t, err)

	assert.Empty(t, empty)
	assert.Equal(t, `C:\a;%USERPROFILE%\b`, got)
	raw, kind, err := k.GetStringValue(pathValue)
	require.NoError(t, err)
	assert.Equal(t, uint32(registry.EXPAND_SZ), kind)
	assert.Equal(t, `C:\a;%USERPROFILE%\b`, raw)
	assert.Contains(t, up.Location(), `\Software\QuiverTest-`)
}

func TestRegistry_Write_KeepsTheValueType(t *testing.T) {
	testCases := []struct {
		name string
		set  func(k registry.Key) error
		want uint32
	}{
		{name: "plain string stays plain", set: func(k registry.Key) error { return k.SetStringValue(pathValue, `C:\a`) }, want: registry.SZ},
		{name: "expandable stays expandable", set: func(k registry.Key) error { return k.SetExpandStringValue(pathValue, `C:\a`) }, want: registry.EXPAND_SZ},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			up, k := scratch(t)
			require.NoError(t, tc.set(k))

			require.NoError(t, up.Write(`C:\a;C:\q`))

			raw, kind, err := k.GetStringValue(pathValue)
			require.NoError(t, err)
			assert.Equal(t, tc.want, kind)
			assert.Equal(t, `C:\a;C:\q`, raw)
		})
	}
}

func TestRegistry_NonStringValueIsAnError(t *testing.T) {
	up, k := scratch(t)
	require.NoError(t, k.SetDWordValue(pathValue, 1))

	_, err := up.Read()
	assert.Error(t, err)
	assert.Error(t, up.Write(`C:\q`))
}

func TestRegistry_MissingKeyIsAnError(t *testing.T) {
	up := &registryUserPath{root: registry.CURRENT_USER, subkey: `Software\QuiverTest-missing-` + t.Name(), area: "QuiverTest"}

	_, err := up.Read()
	assert.Error(t, err)
	assert.Error(t, up.Write(`C:\q`))
}

func TestRegistry_Broadcast_NeverPanics(t *testing.T) {
	up, _ := scratch(t)

	if err := up.Broadcast(); err != nil {
		t.Logf("broadcast not delivered on this desktop: %v", err)
	}
	up.area = "bad\x00area"
	assert.Error(t, up.Broadcast())
}
