package unpack

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDesktopFile_ReadsDesktopEntryGroup(t *testing.T) {
	testCases := []struct {
		name string
		data string
		want desktopEntry
	}{
		{
			name: "name exec and icon",
			data: "[Desktop Entry]\nName=Bruno\nExec=AppRun --no-sandbox %U\nIcon=bruno\nType=Application\n",
			want: desktopEntry{name: "Bruno", exec: "AppRun --no-sandbox %U", icon: "bruno"},
		},
		{
			name: "localized keys are ignored",
			data: "[Desktop Entry]\nName[de]=Brunone\nName=Bruno\nIcon[de]=other\n",
			want: desktopEntry{name: "Bruno"},
		},
		{
			name: "comments blank lines and spacing",
			data: "# a comment\n\n[Desktop Entry]\n  # indented comment\nName = Spaced App \n\nExec=app\n",
			want: desktopEntry{name: "Spaced App", exec: "app"},
		},
		{
			name: "other groups are ignored",
			data: "[Desktop Action new]\nName=New Window\nExec=app --new\n[Desktop Entry]\nName=App\n[X-Extra]\nIcon=nope\n",
			want: desktopEntry{name: "App"},
		},
		{
			name: "keys before any group are ignored",
			data: "Name=Orphan\n[Desktop Entry]\nExec=app\n",
			want: desktopEntry{exec: "app"},
		},
		{
			name: "first occurrence wins",
			data: "[Desktop Entry]\nName=First\nName=Second\n",
			want: desktopEntry{name: "First"},
		},
		{
			name: "lines without equals are ignored",
			data: "[Desktop Entry]\ngarbage line\nName=App\r\n",
			want: desktopEntry{name: "App"},
		},
		{
			name: "empty file",
			data: "",
			want: desktopEntry{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseDesktopFile([]byte(tc.data)))
		})
	}
}

func TestReadCappedDesktopFile_Limits(t *testing.T) {
	head := "[Desktop Entry]\nName=Big\n"
	testCases := []struct {
		name string
		data string
		want desktopEntry
	}{
		{name: "small file", data: head, want: desktopEntry{name: "Big"}},
		{name: "exactly at the cap", data: head + strings.Repeat("#", maxDesktopFileBytes-len(head)), want: desktopEntry{name: "Big"}},
		{name: "over the cap is ignored", data: head + strings.Repeat("#", maxDesktopFileBytes-len(head)+1), want: desktopEntry{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry, err := readCappedDesktopFile(strings.NewReader(tc.data))

			require.NoError(t, err)
			assert.Equal(t, tc.want, entry)
		})
	}
}

func TestReadCappedDesktopFile_ReadError(t *testing.T) {
	_, err := readCappedDesktopFile(iotest.ErrReader(errors.New("disk on fire")))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")
}
