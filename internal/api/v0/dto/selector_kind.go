package dto

import "github.com/rabbytesoftware/quiver.core/internal/domain"

const selectorKindPin = "pin"

// SelectorKindName names a selector kind on the wire. The domain's pin is the
// empty zero value, which a client could not tell apart from a missing field.
func SelectorKindName(k domain.SelectorKind) string {
	if k == domain.SelectorPin {
		return selectorKindPin
	}
	return string(k)
}
