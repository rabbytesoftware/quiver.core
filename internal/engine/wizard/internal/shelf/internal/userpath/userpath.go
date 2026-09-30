package userpath

import (
	"path/filepath"
	"strings"
)

const Separator = ";"

type UserPath interface {
	Read() (string, error)
	Write(
		value string,
	) error
	Broadcast() error
	Location() string
}

func Contains(
	list string,
	dir string,
	sep string,
	fold bool,
) bool {
	for _, entry := range strings.Split(list, sep) {
		if entry != "" && Same(entry, dir, fold) {
			return true
		}
	}
	return false
}

func Same(
	a string,
	b string,
	fold bool,
) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	return a == b || (fold && strings.EqualFold(a, b))
}
