package lnk

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// golden is Encode(Link{Target: `C:\a.exe`, WorkingDir: `C:\`, Description:
// "d"}) laid out field by field from [MS-SHLLINK].
func golden() string {
	return strings.Join([]string{
		// ShellLinkHeader: HeaderSize, LinkCLSID
		"4c000000", "0114020000000000c000000000000046",
		// LinkFlags: HasLinkInfo|HasName|HasWorkingDir|IsUnicode, FileAttributes: NORMAL
		"96000000", "80000000",
		// CreationTime, AccessTime, WriteTime, FileSize, IconIndex
		"0000000000000000", "0000000000000000", "0000000000000000", "00000000", "00000000",
		// ShowCommand: SW_SHOWNORMAL, HotKey, Reserved1-3
		"01000000", "0000", "0000", "00000000", "00000000",
		// LinkInfo: Size 0x54, HeaderSize 0x24, Flags VolumeIDAndLocalBasePath
		"54000000", "24000000", "01000000",
		// VolumeIDOffset, LocalBasePathOffset, CommonNetworkRelativeLinkOffset, CommonPathSuffixOffset
		"24000000", "35000000", "00000000", "3e000000",
		// LocalBasePathOffsetUnicode, CommonPathSuffixOffsetUnicode
		"40000000", "52000000",
		// VolumeID: Size 0x11, DRIVE_FIXED, serial, VolumeLabelOffset 0x10, empty label
		"11000000", "03000000", "00000000", "10000000", "00",
		// LocalBasePath "C:\a.exe", CommonPathSuffix "", padding
		"433a5c612e65786500", "00", "00",
		// LocalBasePathUnicode "C:\a.exe", CommonPathSuffixUnicode ""
		"43003a005c0061002e00650078006500" + "0000", "0000",
		// StringData: NAME_STRING "d", WORKING_DIR "C:\"
		"0100" + "6400", "0300" + "43003a005c00",
		// TerminalBlock
		"00000000",
	}, "")
}

func TestEncode_Golden(t *testing.T) {
	got, err := Encode(Link{Target: `C:\a.exe`, WorkingDir: `C:\`, Description: "d"})

	require.NoError(t, err)
	assert.Equal(t, golden(), hex.EncodeToString(got))
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		link Link
	}{
		{
			name: "owned shortcut with an icon",
			link: Link{
				Target:      `C:\Users\me\.quiver\namespaces\github.com\acme\tool@v1\Tool.exe`,
				WorkingDir:  `C:\Users\me\.quiver\namespaces\github.com\acme\tool@v1`,
				Description: `quiver:github.com/acme/tool|C:\Users\me\.quiver\namespaces\github.com\acme\tool@v1`,
				Icon:        `C:\Users\me\.quiver\namespaces\github.com\acme\tool@v1\tool.ico`,
			},
		},
		{
			name: "non-ascii path",
			link: Link{Target: `C:\Users\José\Café 😀.exe`, WorkingDir: `C:\Users\José`, Description: "quiver:x|y"},
		},
		{
			name: "odd-length ansi path",
			link: Link{Target: `C:\ab.exe`, WorkingDir: `C:\`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Encode(tc.link)
			require.NoError(t, err)

			got, err := Decode(data)

			require.NoError(t, err)
			assert.Equal(t, tc.link, got)
		})
	}
}

func TestEncode_TooLong(t *testing.T) {
	_, err := Encode(Link{Target: `C:\a.exe`, Description: strings.Repeat("x", maxStringChars+1)})

	require.ErrorIs(t, err, errTooLong)
}

// foreign builds a link the way other tools do: an IDList, a LinkInfo with a
// short header (ANSI only) and a suffix, a relative path and arguments, ANSI
// strings, and extra data after them.
func foreign(
	unicode bool,
) []byte {
	flags := uint32(hasLinkTargetIDList | hasLinkInfo | hasName | hasRelativePath | hasWorkingDir | hasArguments | hasIconLocation)
	if unicode {
		flags |= isUnicode
	}
	out := header(flags)
	out = binary.LittleEndian.AppendUint16(out, 4)
	out = append(out, 0xde, 0xad, 0xbe, 0xef)

	info := binary.LittleEndian.AppendUint32(nil, 0)
	for _, v := range []uint32{0x1c, volumeIDAndLocalBasePath, 0x1c, 0x1c, 0, 0x1c + 7} {
		info = binary.LittleEndian.AppendUint32(info, v)
	}
	info = append(info, `C:\dir`...)
	info = append(info, 0, '\\')
	info = append(info, "x.exe"...)
	info = append(info, 0)
	binary.LittleEndian.PutUint32(info, uint32(len(info)))
	out = append(out, info...)

	for _, s := range []string{"quiver:github.com/acme/tool|C:\\wd", `..\x.exe`, `C:\dir`, "--flag", `C:\dir\x.ico`} {
		out = appendString(out, s, unicode)
	}
	return append(out, 0x0c, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 0, 0)
}

func appendString(
	out []byte,
	s string,
	unicode bool,
) []byte {
	if !unicode {
		out = binary.LittleEndian.AppendUint16(out, uint16(len(s)))
		return append(out, s...)
	}
	out = binary.LittleEndian.AppendUint16(out, uint16(len(s)))
	for _, c := range s {
		out = binary.LittleEndian.AppendUint16(out, uint16(c))
	}
	return out
}

func TestDecode_ForeignLinks(t *testing.T) {
	want := Link{Target: `C:\dir\x.exe`, WorkingDir: `C:\dir`, Description: `quiver:github.com/acme/tool|C:\wd`, Icon: `C:\dir\x.ico`}

	for _, unicode := range []bool{true, false} {
		got, err := Decode(foreign(unicode))

		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestLocalPath(t *testing.T) {
	block := func(fields ...uint32) []byte {
		out := []byte{}
		for _, f := range fields {
			out = binary.LittleEndian.AppendUint32(out, f)
		}
		return out
	}

	testCases := []struct {
		name string
		info []byte
		want string
	}{
		{name: "truncated", info: block(0x10, 0x1c), want: ""},
		{name: "network link only", info: block(0x1c, 0x1c, 2, 0, 0, 0x1c, 0), want: ""},
		{name: "offsets past the end", info: block(0x24, 0x24, 1, 0, 0x100, 0, 0x100, 0x200, 0x300), want: ""},
		{name: "unicode header without unicode path falls back to ansi", info: append(block(0x26, 0x24, 1, 0, 0x24, 0, 0x25, 0, 0), 'a', 0), want: "a"},
		{name: "unterminated strings", info: append(block(0x26, 0x1c, 1, 0, 0x1c, 0, 0x1c), 'a', 'b'), want: ""},
		{name: "unterminated unicode string", info: append(block(0x28, 0x24, 1, 0, 0, 0, 0, 0x24, 0x24), 'a', 0, 'b', 0), want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, localPath(tc.info))
		})
	}
}

func TestDecode_Malformed(t *testing.T) {
	valid, err := Encode(Link{Target: `C:\a.exe`, WorkingDir: `C:\`, Description: "quiver:x|y", Icon: `C:\a.ico`})
	require.NoError(t, err)
	wrongSize := append([]byte{}, valid...)
	wrongSize[0] = 0x4d
	wrongCLSID := append([]byte{}, valid...)
	wrongCLSID[4] = 0xff
	hugeInfo := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(hugeInfo[headerSize:], 0xffffffff)
	tinyInfo := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(tinyInfo[headerSize:], 2)
	hugeIDList := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(hugeIDList[0x14:], hasLinkTargetIDList|hasLinkInfo)

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "text file", data: []byte("[InternetShortcut]\r\nURL=https://example.com\r\n")},
		{name: "wrong header size", data: wrongSize},
		{name: "wrong class id", data: wrongCLSID},
		{name: "header only", data: valid[:headerSize-4]},
		{name: "link info past the end", data: hugeInfo},
		{name: "link info smaller than its size field", data: tinyInfo},
		{name: "id list past the end", data: hugeIDList},
	}
	for i := headerSize; i < len(valid)-4; i += 7 {
		testCases = append(testCases, struct {
			name string
			data []byte
		}{name: "truncated", data: valid[:i]})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_, err := Decode(tc.data)
				assert.ErrorIs(t, err, errMalformed)
			})
		})
	}
}

func TestDecode_NeverPanicsOnNoise(t *testing.T) {
	valid, err := Encode(Link{Target: `C:\a.exe`, WorkingDir: `C:\`, Description: "quiver:x|y", Icon: `C:\a.ico`})
	require.NoError(t, err)

	for i := range valid {
		for _, b := range []byte{0x00, 0x7f, 0xff} {
			mutated := append([]byte{}, valid...)
			mutated[i] = b
			assert.NotPanics(t, func() { _, _ = Decode(mutated) }, "byte %d = %#x", i, b)
		}
	}
}
