package domain

import (
	"fmt"
	"strings"
)

const (
	NamespaceSeparator = ":"
)

type Namespace string

func (n Namespace) Validate() error {
	if n == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) != 2 {
		return fmt.Errorf("namespace must be in format QUID:AUID, got: %s", n)
	}

	quid := parts[0]
	auid := parts[1]

	if quid == "" {
		return fmt.Errorf("QUID part of namespace cannot be empty")
	}
	if auid == "" {
		return fmt.Errorf("AUID part of namespace cannot be empty")
	}

	return nil
}

func (n Namespace) GetQUID() string {
	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

func (n Namespace) GetAUID() string {
	parts := strings.Split(string(n), NamespaceSeparator)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func (n Namespace) String() string {
	return string(n)
}
