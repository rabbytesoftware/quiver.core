package step

import (
	"encoding/json"
	"testing"
)

func TestSignalKindConstants(t *testing.T) {
	if SignalKindGraceful != "graceful" {
		t.Errorf("SignalKindGraceful = %q, want graceful", SignalKindGraceful)
	}
	if SignalKindKill != "kill" {
		t.Errorf("SignalKindKill = %q, want kill", SignalKindKill)
	}
	if SignalKindInterrupt != "interrupt" {
		t.Errorf("SignalKindInterrupt = %q, want interrupt", SignalKindInterrupt)
	}
}

func TestOverrideableFromMapNoMutation(t *testing.T) {
	original := map[string]string{
		"default":      "base",
		"linux/amd64":  "linux-specific",
		"darwin/arm64": "darwin-specific",
	}
	expected := make(map[string]string)
	for k, v := range original {
		expected[k] = v
	}

	var o Overrideable[string]
	o.fromMap(original)

	if len(original) != len(expected) {
		t.Errorf("fromMap mutated input map: original len = %d, expected len = %d", len(original), len(expected))
	}
	for k, v := range expected {
		if original[k] != v {
			t.Errorf("fromMap mutated key %q: got %q, expected %q", k, original[k], v)
		}
	}

	if o.Default != "base" {
		t.Errorf("Default = %q, want base", o.Default)
	}
	if o.OSArch["linux/amd64"] != "linux-specific" {
		t.Errorf("OSArch[linux/amd64] = %q, want linux-specific", o.OSArch["linux/amd64"])
	}
}

func TestNewRunStep(t *testing.T) {
	s := NewRunStep("run title", "echo hello", true, "5s", true)

	if s.Type() != StepTypeRun {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeRun)
	}
	if s.Title() != "run title" {
		t.Errorf("Title() = %q, want run title", s.Title())
	}
	if !s.ExitOnFailure() {
		t.Error("ExitOnFailure() = false, want true")
	}
	if s.Command.Default != "echo hello" {
		t.Errorf("Command.Default = %q, want echo hello", s.Command.Default)
	}
	if !s.Elevated.Default {
		t.Errorf("Elevated.Default = %v, want true", s.Elevated.Default)
	}
	if s.Timeout.Default != "5s" {
		t.Errorf("Timeout.Default = %q, want 5s", s.Timeout.Default)
	}
}

func TestNewRunStep_ElevatedFalseByDefault(t *testing.T) {
	s := NewRunStep("title", "cmd", false, "", true)
	if s.Elevated.Default {
		t.Error("Elevated.Default = true, want false")
	}
}

func TestNewFetchStep(t *testing.T) {
	s := NewFetchStep("fetch title", "https://example.com/file", "/tmp/file", "sha256:abc123", "10s", false)

	if s.Type() != StepTypeFetch {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeFetch)
	}
	if s.Title() != "fetch title" {
		t.Errorf("Title() = %q, want fetch title", s.Title())
	}
	if s.ExitOnFailure() {
		t.Error("ExitOnFailure() = true, want false")
	}
	if s.URL.Default != "https://example.com/file" {
		t.Errorf("URL.Default = %q, want https://example.com/file", s.URL.Default)
	}
	if s.To.Default != "/tmp/file" {
		t.Errorf("To.Default = %q, want /tmp/file", s.To.Default)
	}
	if s.Checksum.Default != "sha256:abc123" {
		t.Errorf("Checksum.Default = %q, want sha256:abc123", s.Checksum.Default)
	}
	if s.Timeout.Default != "10s" {
		t.Errorf("Timeout.Default = %q, want 10s", s.Timeout.Default)
	}
}

func TestNewFetchStep_EmptyChecksum(t *testing.T) {
	s := NewFetchStep("title", "http://example.com", "./out", "", "5m", true)
	if s.Checksum.Default != "" {
		t.Errorf("Checksum.Default = %q, want empty", s.Checksum.Default)
	}
}

func TestNewSignalStep(t *testing.T) {
	s := NewSignalStep("signal title", SignalKindGraceful, "3s", false)

	if s.Type() != StepTypeSignal {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeSignal)
	}
	if s.Title() != "signal title" {
		t.Errorf("Title() = %q, want signal title", s.Title())
	}
	if s.ExitOnFailure() {
		t.Error("ExitOnFailure() = true, want false")
	}
	if s.Signal.Default != SignalKindGraceful {
		t.Errorf("Signal.Default = %q, want graceful", s.Signal.Default)
	}
	if s.Timeout.Default != "3s" {
		t.Errorf("Timeout.Default = %q, want 3s", s.Timeout.Default)
	}
}

func TestNewDependenciesStep(t *testing.T) {
	s := NewDependenciesStep("deps title")

	if s.Type() != StepTypeDependencies {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeDependencies)
	}
	if s.Title() != "deps title" {
		t.Errorf("Title() = %q, want deps title", s.Title())
	}
}

func TestDependenciesStep_ExitOnFailure_ReturnsTrue(t *testing.T) {
	s := NewDependenciesStep("Resolve dependencies")
	if !s.ExitOnFailure() {
		t.Error("ExitOnFailure() = false, want true")
	}
}

func TestStepListJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantLen int
		wantErr bool
		check   func(t *testing.T, list StepList)
	}{
		{
			name:    "single run step",
			json:    `[{"type":"run","title":"test","command":"echo hi","elevated":false,"exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				r := list[0].(RunStep)
				if r.Title() != "test" {
					t.Errorf("Title() = %q, want test", r.Title())
				}
				if r.Command.Default != "echo hi" {
					t.Errorf("Command.Default = %q, want echo hi", r.Command.Default)
				}
			},
		},
		{
			name:    "single fetch step",
			json:    `[{"type":"fetch","title":"fetch test","url":"https://example.com","to":"/tmp","checksum":"","exit_on_failure":true,"timeout":"30s"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				f := list[0].(FetchStep)
				if f.Title() != "fetch test" {
					t.Errorf("Title() = %q, want fetch test", f.Title())
				}
				if f.URL.Default != "https://example.com" {
					t.Errorf("URL.Default = %q, want https://example.com", f.URL.Default)
				}
			},
		},
		{
			name:    "mixed step list",
			json:    `[{"type":"run","title":"run","command":"ls","elevated":false,"exit_on_failure":false,"timeout":""},{"type":"fetch","title":"fetch","url":"http://test","to":"/home","checksum":"","exit_on_failure":false,"timeout":""},{"type":"signal","title":"signal","signal":"graceful","exit_on_failure":false,"timeout":"5s"},{"type":"dependencies","title":"deps"}]`,
			wantLen: 4,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				if list[0].Type() != StepTypeRun {
					t.Errorf("step 0 Type = %v, want run", list[0].Type())
				}
				if list[1].Type() != StepTypeFetch {
					t.Errorf("step 1 Type = %v, want fetch", list[1].Type())
				}
				if list[2].Type() != StepTypeSignal {
					t.Errorf("step 2 Type = %v, want signal", list[2].Type())
				}
				if list[3].Type() != StepTypeDependencies {
					t.Errorf("step 3 Type = %v, want dependencies", list[3].Type())
				}
			},
		},
		{
			name:    "unknown step type",
			json:    `[{"type":"unknown","title":"test"}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "invalid json",
			json:    `not valid json`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "malformed step",
			json:    `[{"type":"run","title":"test","command":123}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "empty array",
			json:    `[]`,
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "missing type field",
			json:    `[{"title":"test","command":"echo"}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "invalid json structure",
			json:    `[{"type":"run"`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "single signal step with graceful",
			json:    `[{"type":"signal","title":"sig","signal":"graceful","exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				s := list[0].(SignalStep)
				if s.Signal.Default != SignalKindGraceful {
					t.Errorf("Signal.Default = %q, want graceful", s.Signal.Default)
				}
			},
		},
		{
			name:    "single signal step with kill",
			json:    `[{"type":"signal","title":"sig","signal":"kill","exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				s := list[0].(SignalStep)
				if s.Signal.Default != SignalKindKill {
					t.Errorf("Signal.Default = %q, want kill", s.Signal.Default)
				}
			},
		},
		{
			name:    "single dependencies step",
			json:    `[{"type":"dependencies","title":"install"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				d := list[0].(DependenciesStep)
				if d.Type() != StepTypeDependencies {
					t.Errorf("Type = %v, want dependencies", d.Type())
				}
			},
		},
		{
			name:    "malformed dependencies step",
			json:    `[{"type":"dependencies","title":123}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "type as number",
			json:    `[{"type":123,"title":"test"}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "type as null",
			json:    `[{"type":null,"title":"test"}]`,
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var list StepList
			err := json.Unmarshal([]byte(tt.json), &list)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UnmarshalJSON() error = %v, want nil", err)
			}
			if len(list) != tt.wantLen {
				t.Errorf("len(list) = %v, want %v", len(list), tt.wantLen)
			}
			if tt.check != nil {
				tt.check(t, list)
			}
		})
	}
}

func TestOverrideableStringResolve(t *testing.T) {
	tests := []struct {
		name   string
		o      Overrideable[string]
		osArch string
		want   string
	}{
		{
			name:   "default only, no override",
			o:      Overrideable[string]{Default: "base"},
			osArch: "linux/amd64",
			want:   "base",
		},
		{
			name: "default with matching override",
			o: Overrideable[string]{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64":  "linux-build",
					"darwin/arm64": "darwin-build",
				},
			},
			osArch: "linux/amd64",
			want:   "linux-build",
		},
		{
			name: "default with non-matching override",
			o: Overrideable[string]{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64": "linux-build",
				},
			},
			osArch: "windows/amd64",
			want:   "base",
		},
		{
			name:   "empty default",
			o:      Overrideable[string]{Default: ""},
			osArch: "linux/amd64",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.o.Resolve(tt.osArch)
			if got != tt.want {
				t.Errorf("Resolve(%s) = %v, want %v", tt.osArch, got, tt.want)
			}
		})
	}
}

func TestOverrideableStringJSONUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantDefault string
		wantOSArch  map[string]string
		wantErr     bool
	}{
		{
			name:        "scalar string",
			json:        `"hello"`,
			wantDefault: "hello",
		},
		{
			name:        "object with default only",
			json:        `{"default":"world"}`,
			wantDefault: "world",
		},
		{
			name:        "object with default and overrides",
			json:        `{"default":"base","linux/amd64":"linux-build","darwin/arm64":"darwin-build"}`,
			wantDefault: "base",
			wantOSArch: map[string]string{
				"linux/amd64":  "linux-build",
				"darwin/arm64": "darwin-build",
			},
		},
		{
			name:    "invalid json",
			json:    `not valid`,
			wantErr: true,
		},
		{
			name:    "invalid type array",
			json:    `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name:        "empty string",
			json:        `""`,
			wantDefault: "",
		},
		{
			name:        "empty object",
			json:        `{}`,
			wantDefault: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o Overrideable[string]
			err := json.Unmarshal([]byte(tt.json), &o)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UnmarshalJSON() error = %v, want nil", err)
			}
			if o.Default != tt.wantDefault {
				t.Errorf("Default = %v, want %v", o.Default, tt.wantDefault)
			}
			if len(tt.wantOSArch) == 0 && len(o.OSArch) != 0 {
				t.Errorf("OSArch = %v, want empty map", o.OSArch)
			}
			for k, v := range tt.wantOSArch {
				if o.OSArch[k] != v {
					t.Errorf("OSArch[%s] = %v, want %v", k, o.OSArch[k], v)
				}
			}
		})
	}
}

func TestOverrideableStringJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		o    Overrideable[string]
	}{
		{name: "scalar only", o: Overrideable[string]{Default: "echo hello"}},
		{
			name: "with overrides",
			o: Overrideable[string]{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64":  "linux-build",
					"darwin/arm64": "darwin-build",
				},
			},
		},
		{name: "empty default", o: Overrideable[string]{Default: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.o)
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			var got Overrideable[string]
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}
			if got.Default != tt.o.Default {
				t.Errorf("Default = %q, want %q", got.Default, tt.o.Default)
			}
			for k, v := range tt.o.OSArch {
				if got.OSArch[k] != v {
					t.Errorf("OSArch[%q] = %q, want %q", k, got.OSArch[k], v)
				}
			}
		})
	}
}

func TestStepListJSONRoundTrip(t *testing.T) {
	original := StepList{
		NewRunStep("run", "echo hi", false, "5s", true),
		NewFetchStep("fetch", "https://example.com", "/tmp", "", "30s", false),
		NewSignalStep("signal", SignalKindGraceful, "5s", false),
		NewDependenciesStep("deps"),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got StepList
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if len(got) != len(original) {
		t.Fatalf("len = %d, want %d", len(got), len(original))
	}
	for i, s := range got {
		if s.Type() != original[i].Type() {
			t.Errorf("step %d: Type = %v, want %v", i, s.Type(), original[i].Type())
		}
	}

	r := got[0].(RunStep)
	if r.Title() != "run" || r.Command.Default != "echo hi" {
		t.Errorf("RunStep mismatch: title=%q command=%q", r.Title(), r.Command.Default)
	}

	f := got[1].(FetchStep)
	if f.Title() != "fetch" || f.URL.Default != "https://example.com" {
		t.Errorf("FetchStep mismatch: title=%q url=%q", f.Title(), f.URL.Default)
	}

	sig := got[2].(SignalStep)
	if sig.Signal.Default != SignalKindGraceful {
		t.Errorf("SignalStep.Signal.Default = %q, want graceful", sig.Signal.Default)
	}

	deps := got[3].(DependenciesStep)
	if deps.Title() != "deps" {
		t.Errorf("DependenciesStep.Title() = %q, want deps", deps.Title())
	}
}

func TestRunStep_JSONRoundTrip_WithElevated(t *testing.T) {
	original := NewRunStep("install deps", "sudo apt-get install -y curl", true, "2m", true)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got RunStep
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if got.Elevated.Default != true {
		t.Errorf("Elevated.Default = %v, want true", got.Elevated.Default)
	}
	if got.Command.Default != "sudo apt-get install -y curl" {
		t.Errorf("Command.Default = %q", got.Command.Default)
	}
}

func TestFetchStep_JSONRoundTrip_WithChecksum(t *testing.T) {
	original := NewFetchStep("download binary", "https://example.com/tool", "./tool", "sha256:deadbeef", "5m", true)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	var got FetchStep
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if got.Checksum.Default != "sha256:deadbeef" {
		t.Errorf("Checksum.Default = %q, want sha256:deadbeef", got.Checksum.Default)
	}
}

func TestSignalStep_AllKinds(t *testing.T) {
	kinds := []SignalKind{SignalKindGraceful, SignalKindKill, SignalKindInterrupt}
	for _, kind := range kinds {
		s := NewSignalStep("title", kind, "10s", false)
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("MarshalJSON(%q) error = %v", kind, err)
		}
		var got SignalStep
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("UnmarshalJSON(%q) error = %v", kind, err)
		}
		if got.Signal.Default != kind {
			t.Errorf("Signal.Default = %q, want %q", got.Signal.Default, kind)
		}
	}
}

func TestRunStep_Resolve_UsesOSOverride(t *testing.T) {
	s := RunStep{
		BasicStep: newBasicStep(StepTypeRun, "build", true),
		Command: Overrideable[string]{
			Default: "make build",
			OSArch:  map[string]string{"linux/amd64": "make build-linux"},
		},
		Elevated: Overrideable[bool]{Default: false},
		Timeout:  Overrideable[string]{Default: "10s"},
	}

	got := s.Resolve("linux/amd64").(RunStep)

	if got.Command.Default != "make build-linux" {
		t.Errorf("Command.Default = %q, want make build-linux", got.Command.Default)
	}
	if len(got.Command.OSArch) != 0 {
		t.Errorf("Command.OSArch should be empty after Resolve, got %v", got.Command.OSArch)
	}
}

func TestRunStep_Resolve_FallsBackToDefault(t *testing.T) {
	s := RunStep{
		BasicStep: newBasicStep(StepTypeRun, "build", true),
		Command: Overrideable[string]{
			Default: "make build",
			OSArch:  map[string]string{"linux/amd64": "make build-linux"},
		},
		Elevated: Overrideable[bool]{Default: false},
		Timeout:  Overrideable[string]{Default: "10s"},
	}

	got := s.Resolve("darwin/arm64").(RunStep)

	if got.Command.Default != "make build" {
		t.Errorf("Command.Default = %q, want make build", got.Command.Default)
	}
	if len(got.Command.OSArch) != 0 {
		t.Errorf("Command.OSArch should be empty after Resolve, got %v", got.Command.OSArch)
	}
}

func TestFetchStep_Resolve_UsesOSOverride(t *testing.T) {
	s := FetchStep{
		BasicStep: newBasicStep(StepTypeFetch, "download", true),
		URL: Overrideable[string]{
			Default: "https://example.com/file",
			OSArch:  map[string]string{"linux/amd64": "https://example.com/linux/file"},
		},
		To:       Overrideable[string]{Default: "./file"},
		Checksum: Overrideable[string]{Default: ""},
		Timeout:  Overrideable[string]{Default: "30s"},
	}

	got := s.Resolve("linux/amd64").(FetchStep)

	if got.URL.Default != "https://example.com/linux/file" {
		t.Errorf("URL.Default = %q, want https://example.com/linux/file", got.URL.Default)
	}
	if len(got.URL.OSArch) != 0 {
		t.Errorf("URL.OSArch should be empty after Resolve, got %v", got.URL.OSArch)
	}
}

func TestFetchStep_Resolve_FallsBackToDefault(t *testing.T) {
	s := FetchStep{
		BasicStep: newBasicStep(StepTypeFetch, "download", true),
		URL: Overrideable[string]{
			Default: "https://example.com/file",
			OSArch:  map[string]string{"linux/amd64": "https://example.com/linux/file"},
		},
		To:       Overrideable[string]{Default: "./file"},
		Checksum: Overrideable[string]{Default: ""},
		Timeout:  Overrideable[string]{Default: "30s"},
	}

	got := s.Resolve("windows/amd64").(FetchStep)

	if got.URL.Default != "https://example.com/file" {
		t.Errorf("URL.Default = %q, want https://example.com/file", got.URL.Default)
	}
	if len(got.URL.OSArch) != 0 {
		t.Errorf("URL.OSArch should be empty after Resolve, got %v", got.URL.OSArch)
	}
}

func TestSignalStep_Resolve_UsesOSOverride(t *testing.T) {
	s := SignalStep{
		BasicStep: newBasicStep(StepTypeSignal, "stop", true),
		Signal: Overrideable[SignalKind]{
			Default: SignalKindGraceful,
			OSArch:  map[string]SignalKind{"windows/amd64": SignalKindKill},
		},
		Timeout: Overrideable[string]{Default: "10s"},
	}

	got := s.Resolve("windows/amd64").(SignalStep)

	if got.Signal.Default != SignalKindKill {
		t.Errorf("Signal.Default = %q, want kill", got.Signal.Default)
	}
	if len(got.Signal.OSArch) != 0 {
		t.Errorf("Signal.OSArch should be empty after Resolve, got %v", got.Signal.OSArch)
	}
}

func TestSignalStep_Resolve_FallsBackToDefault(t *testing.T) {
	s := SignalStep{
		BasicStep: newBasicStep(StepTypeSignal, "stop", true),
		Signal: Overrideable[SignalKind]{
			Default: SignalKindGraceful,
			OSArch:  map[string]SignalKind{"windows/amd64": SignalKindKill},
		},
		Timeout: Overrideable[string]{Default: "10s"},
	}

	got := s.Resolve("linux/amd64").(SignalStep)

	if got.Signal.Default != SignalKindGraceful {
		t.Errorf("Signal.Default = %q, want graceful", got.Signal.Default)
	}
	if len(got.Signal.OSArch) != 0 {
		t.Errorf("Signal.OSArch should be empty after Resolve, got %v", got.Signal.OSArch)
	}
}

func TestDependenciesStep_Resolve_ReturnsItself(t *testing.T) {
	s := NewDependenciesStep("install deps")

	got := s.Resolve("linux/amd64").(DependenciesStep)

	if got.Type() != StepTypeDependencies {
		t.Errorf("Type() = %v, want dependencies", got.Type())
	}
	if got.Title() != "install deps" {
		t.Errorf("Title() = %q, want install deps", got.Title())
	}
}
