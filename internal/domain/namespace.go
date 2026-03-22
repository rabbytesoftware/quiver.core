package domain

import (
	"fmt"
	"strings"
)

const (
	NamespaceSeparator = "/"
)

type Namespace string

func (n Namespace) Validate() error {
	if n == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) != 3 && len(parts) != 4 {
		return fmt.Errorf("namespace must be in format domain/user/repo or domain/user/repo/auid, got: %s", n)
	}

	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("namespace segment %d cannot be empty", i)
		}
	}

	return nil
}

func (n Namespace) GetQUID() string {
	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) >= 3 {
		return strings.Join(parts[:3], NamespaceSeparator)
	}
	return string(n)
}

func (n Namespace) GetAUID() string {
	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func (n Namespace) IsQuiverHosted() bool {
	parts := strings.Split(string(n), NamespaceSeparator)
	return len(parts) == 4
}

func (n Namespace) String() string {
	return string(n)
}
