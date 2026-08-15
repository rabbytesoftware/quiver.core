package usecases

import "encoding/json"

// Leaf constrains the value types a configuration field can hold.
type Leaf interface{ bool | int | string }

// Optional distinguishes a JSON field that was absent from one explicitly set
// to null and from one carrying a value. A configuration patch needs all three:
// absent means leave alone, null means restore the default, and a value means
// set it.
type Optional[T Leaf] struct {
	set   bool
	reset bool
	value T
}

// IsSet reports whether the field was present in the request body at all.
func (o Optional[T]) IsSet() bool {
	return o.set
}

// IsReset reports whether the field was present and explicitly null.
func (o Optional[T]) IsReset() bool {
	return o.reset
}

// Value returns the decoded value. It is meaningful only when IsSet reports
// true and IsReset reports false.
func (o Optional[T]) Value() T {
	return o.value
}

// UnmarshalJSON records that the field was present, then decodes it. It is
// never called for an absent field, which is what makes absent and null
// distinguishable.
func (o *Optional[T]) UnmarshalJSON(
	data []byte,
) error {
	o.set = true

	if string(data) == "null" {
		o.reset = true
		return nil
	}

	return json.Unmarshal(data, &o.value)
}
