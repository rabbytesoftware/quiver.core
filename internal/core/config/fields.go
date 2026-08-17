package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Keys lists every configuration field by its dotted path, derived from the
// yaml tags on ConfigData. Adding a setting to ConfigData adds it here, and
// therefore to validation, patching, the restart diff and the overlay, without
// any other code changing.
func Keys() []string {
	var keys []string

	walk(reflect.TypeOf(ConfigData{}), nil, func(key string, _ []int) {
		keys = append(keys, key)
	})

	return keys
}

// Differing lists the dotted keys on which two configurations disagree.
func Differing(
	a, b ConfigData,
) []string {
	var keys []string

	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	walk(reflect.TypeOf(ConfigData{}), nil, func(key string, index []int) {
		if !av.FieldByIndex(index).Equal(bv.FieldByIndex(index)) {
			keys = append(keys, key)
		}
	})

	return keys
}

// RestoreField copies a single field, addressed by its dotted key, from src
// into dst. An unrecognised key is ignored.
func RestoreField(
	dst *ConfigData,
	src ConfigData,
	key string,
) {
	index, ok := indexOf(key)
	if !ok {
		return
	}

	reflect.ValueOf(dst).Elem().FieldByIndex(index).Set(reflect.ValueOf(src).FieldByIndex(index))
}

// SetField decodes a JSON-encoded value into the field addressed by key. A
// JSON null restores the field's value from def, which is how a caller asks
// for a setting to be reset. An unrecognised key is reported, not ignored.
func SetField(
	dst *ConfigData,
	def ConfigData,
	key string,
	raw json.RawMessage,
) error {
	index, ok := indexOf(key)
	if !ok {
		return fmt.Errorf("unknown setting %q", key)
	}

	if string(raw) == "null" {
		RestoreField(dst, def, key)
		return nil
	}

	target := reflect.ValueOf(dst).Elem().FieldByIndex(index)

	value := reflect.New(target.Type())
	if err := json.Unmarshal(raw, value.Interface()); err != nil {
		return fmt.Errorf("must be a %s", target.Type())
	}

	target.Set(value.Elem())

	return nil
}

// walk visits every scalar leaf of a configuration struct, reporting its
// dotted key and the field index path that reaches it.
func walk(
	t reflect.Type,
	prefix []int,
	visit func(key string, index []int),
) {
	for i := range t.NumField() {
		f := t.Field(i)

		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}

		index := append(append([]int{}, prefix...), i)

		if f.Type.Kind() == reflect.Struct {
			walkNested(f.Type, index, name, visit)
			continue
		}

		// Only the leaf kinds a configuration value can hold are visited.
		// Anything else would reach reflect.Value.Equal, which panics on a
		// non-comparable value; TestKeys_ReachEveryLeafOfConfigData fails
		// instead, so an unsupported field is caught in review rather than at
		// runtime.
		if !supportedLeaf(f.Type.Kind()) {
			continue
		}

		visit(name, index)
	}
}

// supportedLeaf reports whether a field kind can be compared, restored and
// decoded by the machinery in this file.
func supportedLeaf(
	kind reflect.Kind,
) bool {
	return kind == reflect.Bool || kind == reflect.Int || kind == reflect.String
}

func walkNested(
	t reflect.Type,
	index []int,
	name string,
	visit func(key string, index []int),
) {
	walk(t, index, func(key string, nested []int) {
		visit(name+"."+key, nested)
	})
}

func indexOf(
	key string,
) ([]int, bool) {
	var (
		found []int
		ok    bool
	)

	walk(reflect.TypeOf(ConfigData{}), nil, func(candidate string, index []int) {
		if candidate == key {
			found, ok = index, true
		}
	})

	return found, ok
}

// Validate reports every configuration field whose value the daemon cannot
// use, keyed by the dotted path the configuration API addresses fields by.
// The rules come from the validate tags on ConfigData.
func Validate(
	data ConfigData,
) []FieldError {
	var errs []FieldError

	if err := newValidator().Struct(data); err != nil {
		var invalid validator.ValidationErrors
		if !errors.As(err, &invalid) {
			return []FieldError{{Key: "", Message: err.Error()}}
		}

		for _, fe := range invalid {
			errs = append(errs, FieldError{Key: dottedKey(fe), Message: describe(fe)})
		}
	}

	return append(errs, portRangeErrors(data)...)
}

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		return name
	})

	// Registration errors are impossible for literal names and non-nil funcs.
	_ = v.RegisterValidation("duration", func(fl validator.FieldLevel) bool {
		return validDuration(fl.Field().String())
	})
	_ = v.RegisterValidation("loglevel", func(fl validator.FieldLevel) bool {
		return validLogLevel(fl.Field().String())
	})
	_ = v.RegisterValidation("quiverhost", func(fl validator.FieldLevel) bool {
		return validHost(fl.Field().String())
	})

	return v
}

// dottedKey drops the struct name that validator puts in front of the
// namespace, leaving the key the configuration API uses.
func dottedKey(
	fe validator.FieldError,
) string {
	_, key, found := strings.Cut(fe.Namespace(), ".")
	if !found {
		return fe.Namespace()
	}

	return key
}

func describe(
	fe validator.FieldError,
) string {
	switch fe.Tag() {
	case "min":
		return fmt.Sprintf("must be at least %s, got %v", fe.Param(), fe.Value())
	case "max":
		return fmt.Sprintf("must be at most %s, got %v", fe.Param(), fe.Value())
	case "duration":
		return fmt.Sprintf("must be a positive duration such as 30s or 5m, got %v", fe.Value())
	case "loglevel":
		return fmt.Sprintf(
			"must be one of debug, trace, info, warn, warning, error, fatal, panic, got %v",
			fe.Value(),
		)
	case "quiverhost":
		return fmt.Sprintf(
			"must be a unix:// or tcp://host:port URI, got %v; recover a running daemon with --host",
			fe.Value(),
		)
	}

	return fmt.Sprintf("fails %s, got %v", fe.Tag(), fe.Value())
}
