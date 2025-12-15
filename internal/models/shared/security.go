package shared

type Security string

const (
	SecurityTrusted   Security = "trusted"
	SecurityUntrusted Security = "untrusted"
)

func (s Security) String() string {
	return string(s)
}

func (s Security) IsTrusted() bool {
	return s == SecurityTrusted
}

func (s Security) IsUntrusted() bool {
	return s == SecurityUntrusted
}
