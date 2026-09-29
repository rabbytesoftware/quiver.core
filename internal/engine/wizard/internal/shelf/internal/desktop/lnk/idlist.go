package lnk

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

const (
	myComputerItem = "\x14\x00\x1f\x50\xe0\x4f\xd0\x20\xea\x3a\x69\x10\xa2\xd8\x08\x00\x2b\x30\x30\x9d"
	driveItemSize  = 0x19
	driveItemType  = 0x2f
	folderItemType = 0x31
	fileItemType   = 0x32
	unicodeName    = 0x04
	fsItemHeader   = 2 + 12
	maxIDListSize  = 0xffff
)

// idList builds the LinkTargetIDList of a drive path: My Computer, the drive,
// then one file-system item per component. The shell resolves a link's target
// from this list; without it, TargetPath reads back empty. An ASCII name is
// stored as an ANSI item, anything else as a Unicode one.
func idList(
	target string,
) []byte {
	drive, rest, ok := drivePath(target)
	if !ok {
		return nil
	}

	items := append([]byte{}, myComputerItem...)
	items = append(items, driveItem(drive)...)
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' })
	for i, part := range parts {
		items = append(items, fsItem(part, i == len(parts)-1)...)
	}
	items = append(items, 0, 0)
	if len(items) > maxIDListSize {
		return nil
	}

	out := binary.LittleEndian.AppendUint16(nil, uint16(len(items))) // #nosec G115 -- bounded by maxIDListSize above
	return append(out, items...)
}

func drivePath(
	target string,
) (string, string, bool) {
	if len(target) < 3 || target[1] != ':' || target[2] != '\\' || !isLetter(target[0]) {
		return "", "", false
	}
	return target[:3], target[3:], true
}

func isLetter(
	c byte,
) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func driveItem(
	drive string,
) []byte {
	out := binary.LittleEndian.AppendUint16(nil, driveItemSize)
	out = append(out, driveItemType)
	out = append(out, drive...)
	return append(out, make([]byte, driveItemSize-len(out))...)
}

func fsItem(
	name string,
	last bool,
) []byte {
	kind := byte(folderItemType)
	if last {
		kind = fileItemType
	}

	encoded := append([]byte(name), 0)
	if !isASCII(name) {
		kind |= unicodeName
		encoded = append(appendUTF16(nil, utf16.Encode([]rune(name))), 0, 0)
	}

	out := binary.LittleEndian.AppendUint16(nil, uint16(fsItemHeader+len(encoded))) // #nosec G115 -- one path component
	out = append(out, kind)
	out = append(out, make([]byte, fsItemHeader-3)...)
	return append(out, encoded...)
}

func isASCII(
	s string,
) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
