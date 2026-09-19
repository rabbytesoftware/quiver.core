package tui

import (
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// CheckPayload reports whether v can be serialized by every output format.
//
// Every command's test must call it on the value its model returns from
// Payload. The contract cannot be expressed in the type system, because
// CommandModel must stay non-generic for Runner to hold heterogeneous models,
// so it is enforced here instead: yaml.v3 panics rather than erroring on a type
// it cannot marshal, and without this check a malformed payload would reach a
// user as a crash rather than failing CI.
//
// This is the one place a recover is appropriate — detecting the panic is the
// function's entire purpose, and it runs in tests rather than on the render
// path.
// json's rejections are in practice a superset of yaml's, so the json check
// catches most bad payloads and reports a real error. The yaml pass exists for
// the residue that only yaml refuses, and the recover catches the panic yaml
// raises instead of returning.
func CheckPayload(v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("payload is not encodable: %v", r)
		}
	}()

	if jsonErr := json.NewEncoder(io.Discard).Encode(v); jsonErr != nil {
		return fmt.Errorf("payload is not json-encodable: %w", jsonErr)
	}

	if yamlErr := yaml.NewEncoder(io.Discard).Encode(v); yamlErr != nil {
		return fmt.Errorf("payload is not yaml-encodable: %w", yamlErr)
	}

	return nil
}
