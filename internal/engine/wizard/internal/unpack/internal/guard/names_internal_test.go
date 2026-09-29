package guard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

func admitAll(
	rules HostRules,
	names ...string,
) error {
	g := &Guard{rules: rules, seen: make(map[string]string)}
	for _, name := range names {
		if err := g.admitName(name); err != nil {
			return err
		}
	}

	return nil
}

func TestAdmitName_WindowsNames(t *testing.T) {
	testCases := []struct {
		name  string
		entry string
	}{
		{name: "device name", entry: "CON"},
		{name: "device name with extension", entry: "nul.txt"},
		{name: "numbered device in a directory", entry: "bin/COM1"},
		{name: "reserved directory", entry: "lpt9/readme"},
		{name: "trailing dot", entry: "dir./file"},
		{name: "trailing space", entry: "file "},
		{name: "alternate data stream", entry: "file:stream"},
		{name: "wildcard", entry: "a*b"},
		{name: "control character", entry: "a\x01b"},
		{name: "backslash separated device", entry: `bin\aux`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, admitAll(HostRules{WindowsNames: true}, tc.entry), models.ErrReservedName)
			assert.ErrorIs(t, admitAll(HostRules{WindowsNames: true, FoldCase: true}, tc.entry), models.ErrReservedName)
			require.NoError(t, admitAll(HostRules{}, tc.entry))
			require.NoError(t, admitAll(HostRules{FoldCase: true}, tc.entry))
		})
	}
}

func TestAdmitName_WindowsNamesAllowsOrdinaryNames(t *testing.T) {
	for _, entry := range []string{"console", "COM0", "COM10", "con-fig/.hidden", "a.b.c", "LPTX"} {
		t.Run(entry, func(t *testing.T) {
			require.NoError(t, admitAll(HostRules{WindowsNames: true}, entry))
		})
	}
}

func TestAdmitName_FoldCase(t *testing.T) {
	testCases := []struct {
		name    string
		entries []string
		collide bool
	}{
		{name: "same file twice", entries: []string{"a/x", "a/x"}},
		{name: "distinct names", entries: []string{"a/x", "a/y", "b"}},
		{name: "files differing in case", entries: []string{"Readme", "README"}, collide: true},
		{name: "parent directories differing in case", entries: []string{"a/x", "A/y"}, collide: true},
		{name: "file against a directory prefix", entries: []string{"lib/x", "LIB"}, collide: true},
		{name: "non-ascii case", entries: []string{"ÉCOLE", "école"}, collide: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, admitAll(HostRules{}, tc.entries...))
			require.NoError(t, admitAll(HostRules{WindowsNames: true}, tc.entries...))
			for _, rules := range []HostRules{{FoldCase: true}, {FoldCase: true, WindowsNames: true}} {
				err := admitAll(rules, tc.entries...)
				if tc.collide {
					assert.ErrorIs(t, err, models.ErrNameCollision)
					continue
				}
				assert.NoError(t, err)
			}
		})
	}
}
