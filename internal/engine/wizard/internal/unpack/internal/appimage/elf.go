package appimage

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

var errNotElf = errors.New("unpack: appimage: not an elf file")

func squashfsOffset(
	src io.ReaderAt,
) (int64, error) {
	ident := make([]byte, elf.EI_NIDENT)
	if _, err := src.ReadAt(ident, 0); err != nil {
		return 0, fmt.Errorf("unpack: appimage: read elf ident: %w", err)
	}
	if string(ident[:len(elf.ELFMAG)]) != elf.ELFMAG {
		return 0, errNotElf
	}

	bo, err := elfByteOrder(elf.Data(ident[elf.EI_DATA]))
	if err != nil {
		return 0, err
	}

	switch class := elf.Class(ident[elf.EI_CLASS]); class {
	case elf.ELFCLASS64:
		return runtimeEnd64(src, bo)
	case elf.ELFCLASS32:
		return runtimeEnd32(src, bo)
	case elf.ELFCLASSNONE:
	}

	return 0, fmt.Errorf("unpack: appimage: unknown elf class %d: %w", ident[elf.EI_CLASS], errNotElf)
}

func elfByteOrder(
	data elf.Data,
) (binary.ByteOrder, error) {
	switch data {
	case elf.ELFDATA2LSB:
		return binary.LittleEndian, nil
	case elf.ELFDATA2MSB:
		return binary.BigEndian, nil
	case elf.ELFDATANONE:
	}

	return nil, fmt.Errorf("unpack: appimage: unknown elf data encoding %d: %w", data, errNotElf)
}

func runtimeEnd64(
	src io.ReaderAt,
	bo binary.ByteOrder,
) (int64, error) {
	var hdr elf.Header64
	if err := readElfStruct(src, 0, bo, &hdr); err != nil {
		return 0, err
	}

	// The squashfs payload is appended right after the ELF runtime, which ends
	// at the later of the section header table and the last program segment.
	end := hdr.Shoff + uint64(hdr.Shentsize)*uint64(hdr.Shnum)
	for i := range uint64(hdr.Phnum) {
		var prog elf.Prog64
		if err := readElfStruct(src, hdr.Phoff+i*uint64(hdr.Phentsize), bo, &prog); err != nil {
			return 0, err
		}
		end = max(end, prog.Off+prog.Filesz)
	}

	return elfOffset(end)
}

func runtimeEnd32(
	src io.ReaderAt,
	bo binary.ByteOrder,
) (int64, error) {
	var hdr elf.Header32
	if err := readElfStruct(src, 0, bo, &hdr); err != nil {
		return 0, err
	}

	end := uint64(hdr.Shoff) + uint64(hdr.Shentsize)*uint64(hdr.Shnum)
	for i := range uint64(hdr.Phnum) {
		var prog elf.Prog32
		if err := readElfStruct(src, uint64(hdr.Phoff)+i*uint64(hdr.Phentsize), bo, &prog); err != nil {
			return 0, err
		}
		end = max(end, uint64(prog.Off)+uint64(prog.Filesz))
	}

	return elfOffset(end)
}

func readElfStruct(
	src io.ReaderAt,
	off uint64,
	bo binary.ByteOrder,
	v any,
) error {
	start, err := elfOffset(off)
	if err != nil {
		return err
	}

	r := io.NewSectionReader(src, start, int64(binary.Size(v)))
	if err := binary.Read(r, bo, v); err != nil {
		return fmt.Errorf("unpack: appimage: read elf header at %d: %w", start, err)
	}

	return nil
}

func elfOffset(
	off uint64,
) (int64, error) {
	if off > math.MaxInt64 {
		return 0, fmt.Errorf("unpack: appimage: elf offset %d out of range: %w", off, errNotElf)
	}

	return int64(off), nil
}
