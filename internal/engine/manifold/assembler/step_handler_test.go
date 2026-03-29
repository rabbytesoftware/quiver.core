package assembler

import "testing"

var knownStepTypes = []string{"run", "fetch", "signal", "dependencies"}

func TestStepHandlerRegistry_AllTypesRegistered(t *testing.T) {
	a := New()

	for _, typeName := range knownStepTypes {
		if _, ok := a.stepHandlers[typeName]; !ok {
			t.Errorf("step type %q not registered", typeName)
		}
	}
}

func TestStepHandlerRegistry_UnknownType(t *testing.T) {
	a := New()

	if _, ok := a.stepHandlers["unknown"]; ok {
		t.Error("expected unknown type to not be registered")
	}
}

func TestStepHandlerRegistry_RunSupportsOverrides(t *testing.T) {
	a := New()

	if !a.stepHandlers["run"].SupportsOverrides() {
		t.Error("run handler should support overrides")
	}
}

func TestStepHandlerRegistry_NonRunDoesNotSupportOverrides(t *testing.T) {
	a := New()

	for _, typeName := range []string{"fetch", "signal", "dependencies"} {
		if a.stepHandlers[typeName].SupportsOverrides() {
			t.Errorf("%q handler should not support overrides", typeName)
		}
	}
}
