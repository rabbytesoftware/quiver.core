package unpack

import (
	"cmp"
	"fmt"
	"io"
	"strings"
)

const (
	desktopEntryGroup   = "[Desktop Entry]"
	maxDesktopFileBytes = 1 << 20
)

type desktopEntry struct {
	name string
	exec string
	icon string
}

func readCappedDesktopFile(
	r io.Reader,
) (desktopEntry, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDesktopFileBytes+1))
	if err != nil {
		return desktopEntry{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}
	if len(data) > maxDesktopFileBytes {
		return desktopEntry{}, nil
	}

	return parseDesktopFile(data), nil
}

func parseDesktopFile(
	data []byte,
) desktopEntry {
	var entry desktopEntry
	inGroup := false
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			inGroup = line == desktopEntryGroup
		case inGroup:
			entry.set(line)
		}
	}

	return entry
}

func (e *desktopEntry) set(
	line string,
) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return
	}

	value = strings.TrimSpace(value)
	switch strings.TrimSpace(key) {
	case "Name":
		e.name = cmp.Or(e.name, value)
	case "Exec":
		e.exec = cmp.Or(e.exec, value)
	case "Icon":
		e.icon = cmp.Or(e.icon, value)
	}
}
