package domain

import (
	"fmt"
	"strings"
)

const (
	NamespaceSeparator = "/"
)

type Namespace string

func (n Namespace) BareNamespace() Namespace {
	s := string(n)
	if idx := strings.IndexByte(s, '@'); idx >= 0 {
		return Namespace(s[:idx])
	}
	return n
}

func (n Namespace) Ref() string {
	s := string(n)
	if idx := strings.IndexByte(s, '@'); idx >= 0 {
		return s[idx+1:]
	}
	return ""
}

func (n Namespace) IsGlob() bool {
	return strings.Contains(n.Ref(), "*")
}

func (n Namespace) Validate() error {
	if n == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	bare := string(n.BareNamespace())
	parts := strings.Split(bare, NamespaceSeparator)
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
	parts := strings.Split(string(n.BareNamespace()), NamespaceSeparator)
	if len(parts) >= 3 {
		return strings.Join(parts[:3], NamespaceSeparator)
	}
	return string(n.BareNamespace())
}

func (n Namespace) GetAUID() string {
	parts := strings.Split(string(n.BareNamespace()), NamespaceSeparator)
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

func (n Namespace) IsQuiverHosted() bool {
	parts := strings.Split(string(n.BareNamespace()), NamespaceSeparator)
	return len(parts) == 4
}

// WithRef returns a new Namespace with the given ref replacing any existing ref.
// If ref is empty, the bare namespace is returned with no trailing '@'.
func (n Namespace) WithRef(ref string) Namespace {
	bare := n.BareNamespace()
	if ref == "" {
		return bare
	}
	return Namespace(string(bare) + "@" + ref)
}

func (n Namespace) String() string {
	return string(n)
}

func (n Namespace) Domain() string {
	parts := strings.SplitN(string(n.BareNamespace()), NamespaceSeparator, 2)
	return parts[0]
}

func (n Namespace) CloneURL() string {
	parts := strings.Split(string(n.BareNamespace()), NamespaceSeparator)
	if len(parts) < 3 {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/%s", parts[0], parts[1], parts[2])
}
