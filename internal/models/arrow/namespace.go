package arrow

import "strings"

type ArrowNamespace string

func (a ArrowNamespace) IsValid() bool {
	parts := strings.Split(string(a), "@")
	if len(parts) != 2 {
		return false
	}

	// Check that both parts are non-empty
	if parts[0] == "" || parts[1] == "" {
		return false
	}

	return true
}

func (a ArrowNamespace) String() string {
	return string(a)
}
