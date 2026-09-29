package lnk

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

type Link struct {
	Target      string
	WorkingDir  string
	Description string
	Icon        string
}

const (
	headerSize         = 0x4c
	linkInfoHeaderSize = 0x24
	volumeIDHeaderSize = 0x10
	maxStringChars     = 0xffff

	linkCLSID = "\x01\x14\x02\x00\x00\x00\x00\x00\xc0\x00\x00\x00\x00\x00\x00\x46"

	hasLinkTargetIDList = 1 << 0
	hasLinkInfo         = 1 << 1
	hasName             = 1 << 2
	hasRelativePath     = 1 << 3
	hasWorkingDir       = 1 << 4
	hasArguments        = 1 << 5
	hasIconLocation     = 1 << 6
	isUnicode           = 1 << 7

	fileAttributeNormal      = 0x80
	swShowNormal             = 1
	driveFixed               = 3
	volumeIDAndLocalBasePath = 1
)

var (
	errMalformed = errors.New("malformed shell link")
	errTooLong   = errors.New("shell link string too long")
)

// Encode writes a shell link that resolves through its LinkInfo alone:
// header, a VolumeID/LocalBasePath LinkInfo with the Unicode path, the
// StringData and a terminal ExtraData block.
func Encode(
	l Link,
) ([]byte, error) {
	flags := uint32(hasLinkInfo | hasName | hasWorkingDir | isUnicode)
	strs := []string{l.Description, l.WorkingDir}
	if l.Icon != "" {
		flags |= hasIconLocation
		strs = append(strs, l.Icon)
	}

	out := header(flags)
	out = append(out, linkInfo(l.Target)...)
	for _, s := range strs {
		units := utf16.Encode([]rune(s))
		if len(units) > maxStringChars {
			return nil, errTooLong
		}
		out = binary.LittleEndian.AppendUint16(out, uint16(len(units))) // #nosec G115 -- bounded by maxStringChars above
		out = appendUTF16(out, units)
	}
	return binary.LittleEndian.AppendUint32(out, 0), nil
}

func header(
	flags uint32,
) []byte {
	out := binary.LittleEndian.AppendUint32(nil, headerSize)
	out = append(out, linkCLSID...)
	out = binary.LittleEndian.AppendUint32(out, flags)
	out = binary.LittleEndian.AppendUint32(out, fileAttributeNormal)
	out = append(out, make([]byte, 3*8+4+4)...)
	out = binary.LittleEndian.AppendUint32(out, swShowNormal)
	return append(out, make([]byte, 2+2+4+4)...)
}

func linkInfo(
	target string,
) []byte {
	volumeID := binary.LittleEndian.AppendUint32(nil, volumeIDHeaderSize+1)
	volumeID = binary.LittleEndian.AppendUint32(volumeID, driveFixed)
	volumeID = binary.LittleEndian.AppendUint32(volumeID, 0)
	volumeID = binary.LittleEndian.AppendUint32(volumeID, volumeIDHeaderSize)
	volumeID = append(volumeID, 0)

	ansiPath := append(ansi(target), 0)
	body := append(append(volumeID, ansiPath...), 0)
	if (linkInfoHeaderSize+len(body))%2 != 0 {
		body = append(body, 0)
	}
	unicodeOffset := linkInfoHeaderSize + len(body)
	body = append(appendUTF16(body, utf16.Encode([]rune(target))), 0, 0, 0, 0)

	suffixOffset := linkInfoHeaderSize + len(volumeID) + len(ansiPath)
	fields := []int{
		linkInfoHeaderSize + len(body),
		linkInfoHeaderSize,
		volumeIDAndLocalBasePath,
		linkInfoHeaderSize,
		linkInfoHeaderSize + len(volumeID),
		0,
		suffixOffset,
		unicodeOffset,
		linkInfoHeaderSize + len(body) - 2,
	}
	out := make([]byte, 0, linkInfoHeaderSize+len(body))
	for _, v := range fields {
		out = binary.LittleEndian.AppendUint32(out, uint32(v)) // #nosec G115 -- offsets inside one link of bounded strings
	}
	return append(out, body...)
}

// Decode reads a shell link written by any tool without ever panicking: every
// read is bounds-checked and anything inconsistent is errMalformed.
func Decode(
	data []byte,
) (Link, error) {
	r := &reader{data: data}
	size := r.u32()
	clsid := r.bytes(16)
	flags := r.u32()
	if r.err != nil || size != headerSize || string(clsid) != linkCLSID {
		return Link{}, errMalformed
	}
	r.seek(headerSize)

	if flags&hasLinkTargetIDList != 0 {
		r.skip(int(r.u16()))
	}

	var l Link
	if flags&hasLinkInfo != 0 {
		l.Target = localPath(r.block())
	}

	unicode := flags&isUnicode != 0
	for _, field := range []struct {
		flag uint32
		dest *string
	}{
		{hasName, &l.Description},
		{hasRelativePath, nil},
		{hasWorkingDir, &l.WorkingDir},
		{hasArguments, nil},
		{hasIconLocation, &l.Icon},
	} {
		if flags&field.flag == 0 {
			continue
		}
		s := r.str(unicode)
		if field.dest != nil {
			*field.dest = s
		}
	}

	if r.err != nil {
		return Link{}, r.err
	}
	return l, nil
}

func localPath(
	info []byte,
) string {
	r := &reader{data: info}
	r.seek(4)
	headerLen := r.u32()
	flags := r.u32()
	r.seek(16)
	baseOffset := r.u32()
	r.seek(24)
	suffixOffset := r.u32()
	if r.err != nil || flags&volumeIDAndLocalBasePath == 0 {
		return ""
	}
	if headerLen < linkInfoHeaderSize {
		return r.cstring(baseOffset) + r.cstring(suffixOffset)
	}

	baseUnicode := r.u32()
	suffixUnicode := r.u32()
	if r.err != nil || baseUnicode == 0 {
		return r.cstring(baseOffset) + r.cstring(suffixOffset)
	}
	return r.wstring(baseUnicode) + r.wstring(suffixUnicode)
}

func ansi(
	s string,
) []byte {
	out := make([]byte, 0, len(s))
	for _, c := range s {
		if c >= 0x80 {
			c = '?'
		}
		out = append(out, byte(c))
	}
	return out
}

func appendUTF16(
	out []byte,
	units []uint16,
) []byte {
	for _, u := range units {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return out
}

func latin1(
	b []byte,
) string {
	runes := make([]rune, len(b))
	for i, c := range b {
		runes[i] = rune(c)
	}
	return string(runes)
}

type reader struct {
	data []byte
	pos  int
	err  error
}

func (r *reader) bytes(
	n int,
) []byte {
	if r.err != nil || n < 0 || n > len(r.data)-r.pos {
		r.err = errMalformed
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *reader) u16() uint16 {
	b := r.bytes(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *reader) u32() uint32 {
	b := r.bytes(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *reader) skip(
	n int,
) {
	r.bytes(n)
}

func (r *reader) seek(
	pos int,
) {
	if pos > len(r.data) {
		r.err = errMalformed
		return
	}
	r.pos = pos
}

func (r *reader) block() []byte {
	start := r.pos
	size := r.u32()
	if r.err != nil || size < 4 || int64(size) > int64(len(r.data)-start) {
		r.err = errMalformed
		return nil
	}
	r.pos = start + int(size)
	return r.data[start:r.pos]
}

func (r *reader) str(
	unicode bool,
) string {
	count := int(r.u16())
	if !unicode {
		return latin1(r.bytes(count))
	}
	return string(utf16.Decode(units(r.bytes(2 * count))))
}

func (r *reader) cstring(
	offset uint32,
) string {
	if int64(offset) >= int64(len(r.data)) {
		return ""
	}
	tail := r.data[offset:]
	for i, c := range tail {
		if c == 0 {
			return latin1(tail[:i])
		}
	}
	return ""
}

func (r *reader) wstring(
	offset uint32,
) string {
	if int64(offset) >= int64(len(r.data)) {
		return ""
	}
	all := units(r.data[offset:])
	for i, u := range all {
		if u == 0 {
			return string(utf16.Decode(all[:i]))
		}
	}
	return ""
}

func units(
	raw []byte,
) []uint16 {
	out := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		out = append(out, binary.LittleEndian.Uint16(raw[i:]))
	}
	return out
}
