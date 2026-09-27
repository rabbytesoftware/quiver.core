package unpack

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func elf64WithProgram(
	bo binary.ByteOrder,
	progOff uint64,
	progSize uint64,
) []byte {
	data := make([]byte, 256)
	copy(data, unpacktest.ElfHeader(2))
	data[5] = 1
	if bo == binary.BigEndian {
		data[5] = 2
	}
	bo.PutUint64(data[0x20:], 64)
	bo.PutUint64(data[0x28:], 120)
	bo.PutUint16(data[0x36:], 56)
	bo.PutUint16(data[0x38:], 1)
	bo.PutUint16(data[0x3A:], 64)
	bo.PutUint16(data[0x3C:], 1)
	bo.PutUint64(data[64+0x08:], progOff)
	bo.PutUint64(data[64+0x20:], progSize)

	return data
}

func elf32WithProgram(
	progOff uint32,
	progSize uint32,
) []byte {
	data := make([]byte, 128)
	copy(data, "\x7fELF\x01\x01\x01")
	le := binary.LittleEndian
	le.PutUint32(data[0x1C:], 52)
	le.PutUint32(data[0x20:], 52)
	le.PutUint16(data[0x2A:], 32)
	le.PutUint16(data[0x2C:], 1)
	le.PutUint16(data[0x2E:], 40)
	le.PutUint16(data[0x30:], 2)
	le.PutUint32(data[52+0x04:], progOff)
	le.PutUint32(data[52+0x10:], progSize)

	return data
}

func TestSquashfsOffset_HeaderOnlyIsEndOfSectionTable(t *testing.T) {
	off, err := squashfsOffset(bytes.NewReader(unpacktest.ElfHeader(2)))

	require.NoError(t, err)
	assert.Equal(t, int64(64), off)
}

func TestSquashfsOffset_ProgramHeaders(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
		want int64
	}{
		{name: "64-bit segment past the section table raises the offset", data: elf64WithProgram(binary.LittleEndian, 100, 400), want: 500},
		{name: "64-bit segment before the section table keeps it", data: elf64WithProgram(binary.LittleEndian, 0, 10), want: 184},
		{name: "64-bit big endian", data: elf64WithProgram(binary.BigEndian, 100, 400), want: 500},
		{name: "32-bit segment past the section table raises the offset", data: elf32WithProgram(100, 300), want: 400},
		{name: "32-bit segment before the section table keeps it", data: elf32WithProgram(0, 10), want: 132},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			off, err := squashfsOffset(bytes.NewReader(tc.data))

			require.NoError(t, err)
			assert.Equal(t, tc.want, off)
		})
	}
}

func TestSquashfsOffset_Errors(t *testing.T) {
	truncated64 := elf64WithProgram(binary.LittleEndian, 0, 0)
	binary.LittleEndian.PutUint64(truncated64[0x20:], 1<<20)
	truncated32 := elf32WithProgram(0, 0)
	binary.LittleEndian.PutUint32(truncated32[0x1C:], 1<<20)
	huge := unpacktest.ElfHeader(2)
	binary.LittleEndian.PutUint64(huge[0x28:], 1<<63)
	hugeProg := elf64WithProgram(binary.LittleEndian, 0, 0)
	binary.LittleEndian.PutUint64(hugeProg[0x20:], 1<<63)
	badClass := unpacktest.ElfHeader(2)
	badClass[4] = 7
	badData := unpacktest.ElfHeader(2)
	badData[5] = 9

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "not an elf file", data: []byte("MZ this is not an elf file at all")},
		{name: "shorter than the elf ident", data: []byte("\x7fELF")},
		{name: "unknown class", data: badClass},
		{name: "unknown data encoding", data: badData},
		{name: "truncated 64-bit header", data: unpacktest.ElfHeader(2)[:40]},
		{name: "truncated 32-bit header", data: []byte("\x7fELF\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")},
		{name: "64-bit program header out of range", data: truncated64},
		{name: "32-bit program header out of range", data: truncated32},
		{name: "offset beyond int64", data: huge},
		{name: "program header offset beyond int64", data: hugeProg},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := squashfsOffset(bytes.NewReader(tc.data))

			require.Error(t, err)
		})
	}
}
