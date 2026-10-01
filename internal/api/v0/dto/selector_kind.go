package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

const selectorKindPin = "pin"

// SelectorKindName names a selector kind on the wire: its family, since the
// refinement is how the row follows its selector, not what the client picks.
// The domain's pin is the empty zero value, which a client could not tell
// apart from a missing field.
func SelectorKindName(k domain.SelectorKind) string {
	family := k.Family()
	if family == domain.SelectorPin {
		return selectorKindPin
	}
	return string(family)
}
