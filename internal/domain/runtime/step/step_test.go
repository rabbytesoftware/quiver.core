package step

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalKindConstants(t *testing.T) {
	assert.Equal(t, SignalKind("graceful"), SignalKindGraceful)
	assert.Equal(t, SignalKind("kill"), SignalKindKill)
	assert.Equal(t, SignalKind("interrupt"), SignalKindInterrupt)
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

	assert.Len(t, original, len(expected), "fromMap mutated input map")
	for k, v := range expected {
		assert.Equal(t, v, original[k], "fromMap mutated key %q", k)
	}

	assert.Equal(t, "base", o.Default)
	assert.Equal(t, "linux-specific", o.OSArch["linux/amd64"])
}

func TestNewRunStep(t *testing.T) {
	s := NewRunStep("run title", "echo hello", true, "5s", true)

	assert.Equal(t, StepTypeRun, s.Type())
	assert.Equal(t, "run title", s.Title())
	assert.True(t, s.ExitOnFailure())
	assert.Equal(t, "echo hello", s.Command.Default)
	assert.True(t, s.Elevated.Default)
	assert.Equal(t, "5s", s.Timeout.Default)
}

func TestNewRunStep_ElevatedFalseByDefault(t *testing.T) {
	s := NewRunStep("title", "cmd", false, "", true)
	assert.False(t, s.Elevated.Default)
}

func TestNewFetchStep(t *testing.T) {
	s := NewFetchStep("fetch title", "https://example.com/file", "/tmp/file", "sha256:abc123", "10s", false)

	assert.Equal(t, StepTypeFetch, s.Type())
	assert.Equal(t, "fetch title", s.Title())
	assert.False(t, s.ExitOnFailure())
	assert.Equal(t, "https://example.com/file", s.URL.Default)
	assert.Equal(t, "/tmp/file", s.To.Default)
	assert.Equal(t, "sha256:abc123", s.Checksum.Default)
	assert.Equal(t, "10s", s.Timeout.Default)
}

func TestNewFetchStep_EmptyChecksum(t *testing.T) {
	s := NewFetchStep("title", "http://example.com", "./out", "", "5m", true)
	assert.Equal(t, "", s.Checksum.Default)
}

func TestNewFromToSteps(t *testing.T) {
	extract := NewExtractStep("step title", "/tmp/in", "/tmp/out", "10s", true)
	portable := NewPortableStep("step title", "/tmp/in", "/tmp/out", "10s", true)

	testCases := []struct {
		name    string
		step    Step
		kind    StepType
		from    Overrideable[string]
		to      Overrideable[string]
		timeout Overrideable[string]
	}{
		{name: "extract", step: extract, kind: StepTypeExtract, from: extract.From, to: extract.To, timeout: extract.Timeout},
		{name: "portable", step: portable, kind: StepTypePortable, from: portable.From, to: portable.To, timeout: portable.Timeout},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.kind, tc.step.Type())
			assert.Equal(t, "step title", tc.step.Title())
			assert.True(t, tc.step.ExitOnFailure())
			assert.Equal(t, "/tmp/in", tc.from.Default)
			assert.Equal(t, "/tmp/out", tc.to.Default)
			assert.Equal(t, "10s", tc.timeout.Default)
		})
	}
}

func TestNewSignalStep(t *testing.T) {
	s := NewSignalStep("signal title", SignalKindGraceful, "3s", false)

	assert.Equal(t, StepTypeSignal, s.Type())
	assert.Equal(t, "signal title", s.Title())
	assert.False(t, s.ExitOnFailure())
	assert.Equal(t, SignalKindGraceful, s.Signal.Default)
	assert.Equal(t, "3s", s.Timeout.Default)
}

func TestNewDependenciesStep(t *testing.T) {
	s := NewDependenciesStep("deps title")

	assert.Equal(t, StepTypeDependencies, s.Type())
	assert.Equal(t, "deps title", s.Title())
}

func TestDependenciesStep_ExitOnFailure_ReturnsTrue(t *testing.T) {
	s := NewDependenciesStep("Resolve dependencies")
	assert.True(t, s.ExitOnFailure())
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
				assert.Equal(t, "test", r.Title())
				assert.Equal(t, "echo hi", r.Command.Default)
			},
		},
		{
			name:    "single fetch step",
			json:    `[{"type":"fetch","title":"fetch test","url":"https://example.com","to":"/tmp","checksum":"","exit_on_failure":true,"timeout":"30s"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				f := list[0].(FetchStep)
				assert.Equal(t, "fetch test", f.Title())
				assert.Equal(t, "https://example.com", f.URL.Default)
			},
		},
		{
			name:    "mixed step list",
			json:    `[{"type":"run","title":"run","command":"ls","elevated":false,"exit_on_failure":false,"timeout":""},{"type":"fetch","title":"fetch","url":"http://test","to":"/home","checksum":"","exit_on_failure":false,"timeout":""},{"type":"signal","title":"signal","signal":"graceful","exit_on_failure":false,"timeout":"5s"},{"type":"dependencies","title":"deps"}]`,
			wantLen: 4,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				assert.Equal(t, StepTypeRun, list[0].Type())
				assert.Equal(t, StepTypeFetch, list[1].Type())
				assert.Equal(t, StepTypeSignal, list[2].Type())
				assert.Equal(t, StepTypeDependencies, list[3].Type())
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
				assert.Equal(t, SignalKindGraceful, s.Signal.Default)
			},
		},
		{
			name:    "single signal step with kill",
			json:    `[{"type":"signal","title":"sig","signal":"kill","exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				s := list[0].(SignalStep)
				assert.Equal(t, SignalKindKill, s.Signal.Default)
			},
		},
		{
			name:    "single dependencies step",
			json:    `[{"type":"dependencies","title":"install"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				d := list[0].(DependenciesStep)
				assert.Equal(t, StepTypeDependencies, d.Type())
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

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, list, tt.wantLen)
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
			assert.Equal(t, tt.want, got)
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

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDefault, o.Default)
			if len(tt.wantOSArch) == 0 {
				assert.Empty(t, o.OSArch)
			}
			for k, v := range tt.wantOSArch {
				assert.Equal(t, v, o.OSArch[k])
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
			require.NoError(t, err)

			var got Overrideable[string]
			require.NoError(t, json.Unmarshal(data, &got))

			assert.Equal(t, tt.o.Default, got.Default)
			for k, v := range tt.o.OSArch {
				assert.Equal(t, v, got.OSArch[k])
			}
		})
	}
}

func TestStepListJSONRoundTrip(t *testing.T) {
	original := StepList{
		NewRunStep("run", "echo hi", false, "5s", true),
		NewFetchStep("fetch", "https://example.com", "/tmp", "", "30s", false),
		NewExtractStep("extract", "/tmp/archive.tar.gz", "/tmp/out", "1m", false),
		NewPortableStep("portable", "/tmp/bruno.AppImage", "/tmp/out", "1m", false),
		NewSignalStep("signal", SignalKindGraceful, "5s", false),
		NewDependenciesStep("deps"),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got StepList
	require.NoError(t, json.Unmarshal(data, &got))

	require.Len(t, got, len(original))
	for i, s := range got {
		assert.Equal(t, original[i].Type(), s.Type(), "step %d type mismatch", i)
	}

	r := got[0].(RunStep)
	assert.Equal(t, "run", r.Title())
	assert.Equal(t, "echo hi", r.Command.Default)

	f := got[1].(FetchStep)
	assert.Equal(t, "fetch", f.Title())
	assert.Equal(t, "https://example.com", f.URL.Default)

	e := got[2].(ExtractStep)
	assert.Equal(t, "extract", e.Title())
	assert.Equal(t, "/tmp/archive.tar.gz", e.From.Default)

	p := got[3].(PortableStep)
	assert.Equal(t, "portable", p.Title())
	assert.Equal(t, "/tmp/bruno.AppImage", p.From.Default)

	sig := got[4].(SignalStep)
	assert.Equal(t, SignalKindGraceful, sig.Signal.Default)

	deps := got[5].(DependenciesStep)
	assert.Equal(t, "deps", deps.Title())
}

func TestRunStep_JSONRoundTrip_WithElevated(t *testing.T) {
	original := NewRunStep("install deps", "sudo apt-get install -y curl", true, "2m", true)

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got RunStep
	require.NoError(t, json.Unmarshal(data, &got))

	assert.True(t, got.Elevated.Default)
	assert.Equal(t, "sudo apt-get install -y curl", got.Command.Default)
}

func TestRunStep_JSONRoundTrip_WithUI(t *testing.T) {
	ui := UIOptions{Title: "Chat", Path: "/app", Listen: []string{"unix"}}
	original := NewRunStep("start", "./chat", false, "", true).WithUI(ui)

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got RunStep
	require.NoError(t, json.Unmarshal(data, &got))

	require.NotNil(t, got.UI)
	assert.Equal(t, ui, *got.UI)
}

func TestRunStep_JSONOmitsAbsentUI(t *testing.T) {
	data, err := json.Marshal(NewRunStep("start", "./chat", false, "", true))
	require.NoError(t, err)

	assert.NotContains(t, string(data), `"ui"`)
}

func TestRunStep_Resolve_KeepsUI(t *testing.T) {
	s := NewRunStep("start", "./chat", false, "", true).WithUI(UIOptions{Static: "./dist"})

	got := s.Resolve("linux/amd64").(RunStep)

	require.NotNil(t, got.UI)
	assert.Equal(t, "./dist", got.UI.Static)
}

func TestUIOptions_Listens(t *testing.T) {
	assert.True(t, UIOptions{}.Listens())
	assert.True(t, UIOptions{Listen: []string{"unix"}}.Listens())
	assert.False(t, UIOptions{Static: "./dist"}.Listens())
}

func TestPortableStep_JSONRoundTrip(t *testing.T) {
	original := NewPortableStep("install app", "./bruno.AppImage", "./out", "2m", true)
	original.Name = "bruno"

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got PortableStep
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "./bruno.AppImage", got.From.Default)
	assert.Equal(t, "./out", got.To.Default)
	assert.Equal(t, "2m", got.Timeout.Default)
	assert.Equal(t, "bruno", got.Name)
	assert.True(t, got.ExitOnFailure())
}

func TestPortableStep_Resolve_KeepsName(t *testing.T) {
	s := NewPortableStep("install app", "./tool.download", "./tool", "", true)
	s.Name = "tool.exe"

	got := s.Resolve("windows/amd64").(PortableStep)

	assert.Equal(t, "tool.exe", got.Name)
}

func TestFromToSteps_Resolve(t *testing.T) {
	overrides := Overrideable[string]{Default: "./default", OSArch: map[string]string{"windows/amd64": "./windows"}}
	extract := ExtractStep{BasicStep: newBasicStep(StepTypeExtract, "unpack", true), From: overrides}
	portable := PortableStep{BasicStep: newBasicStep(StepTypePortable, "install", true), From: overrides}

	testCases := []struct {
		name string
		step Step
		os   string
		want string
	}{
		{name: "extract uses the os override", step: extract, os: "windows/amd64", want: "./windows"},
		{name: "extract falls back to the default", step: extract, os: "linux/amd64", want: "./default"},
		{name: "portable uses the os override", step: portable, os: "windows/amd64", want: "./windows"},
		{name: "portable falls back to the default", step: portable, os: "linux/amd64", want: "./default"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var from Overrideable[string]
			switch got := tc.step.Resolve(tc.os).(type) {
			case ExtractStep:
				from = got.From
			case PortableStep:
				from = got.From
			}

			assert.Equal(t, tc.want, from.Default)
			assert.Empty(t, from.OSArch)
		})
	}
}

func TestFromToSteps_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var extract ExtractStep
	var portable PortableStep

	require.Error(t, extract.UnmarshalJSON([]byte(`{invalid json`)))
	require.Error(t, portable.UnmarshalJSON([]byte(`{invalid json`)))
}

func TestFetchStep_JSONRoundTrip_WithChecksum(t *testing.T) {
	original := NewFetchStep("download binary", "https://example.com/tool", "./tool", "sha256:deadbeef", "5m", true)

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var got FetchStep
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "sha256:deadbeef", got.Checksum.Default)
}

func TestSignalStep_AllKinds(t *testing.T) {
	kinds := []SignalKind{SignalKindGraceful, SignalKindKill, SignalKindInterrupt}
	for _, kind := range kinds {
		s := NewSignalStep("title", kind, "10s", false)
		data, err := json.Marshal(s)
		require.NoError(t, err, "MarshalJSON(%q)", kind)

		var got SignalStep
		require.NoError(t, json.Unmarshal(data, &got), "UnmarshalJSON(%q)", kind)

		assert.Equal(t, kind, got.Signal.Default)
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

	assert.Equal(t, "make build-linux", got.Command.Default)
	assert.Empty(t, got.Command.OSArch, "Command.OSArch should be empty after Resolve")
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

	assert.Equal(t, "make build", got.Command.Default)
	assert.Empty(t, got.Command.OSArch, "Command.OSArch should be empty after Resolve")
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

	assert.Equal(t, "https://example.com/linux/file", got.URL.Default)
	assert.Empty(t, got.URL.OSArch, "URL.OSArch should be empty after Resolve")
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

	assert.Equal(t, "https://example.com/file", got.URL.Default)
	assert.Empty(t, got.URL.OSArch, "URL.OSArch should be empty after Resolve")
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

	assert.Equal(t, SignalKindKill, got.Signal.Default)
	assert.Empty(t, got.Signal.OSArch, "Signal.OSArch should be empty after Resolve")
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

	assert.Equal(t, SignalKindGraceful, got.Signal.Default)
	assert.Empty(t, got.Signal.OSArch, "Signal.OSArch should be empty after Resolve")
}

func TestDerefStep_UnknownType_ReturnsSelf(t *testing.T) {
	// Pass something that doesn't match any known pointer type.
	// derefStep's default branch returns the step as-is.
	s := NewRunStep("run", "echo hi", false, "5s", true)
	got := derefStep(s)
	assert.Equal(t, s, got)
}

func TestFetchStep_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var s FetchStep
	err := s.UnmarshalJSON([]byte(`{invalid json`))
	require.Error(t, err)
}

func TestFetchStep_UnmarshalJSON_Success(t *testing.T) {
	data := []byte(`{"type":"fetch","title":"dl","url":"https://example.com","to":"/tmp","checksum":"abc","exit_on_failure":true,"timeout":"10s"}`)
	var s FetchStep
	err := s.UnmarshalJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", s.URL.Default)
	assert.Equal(t, "/tmp", s.To.Default)
	assert.Equal(t, "abc", s.Checksum.Default)
	assert.True(t, s.ExitOnFailure())
}

func TestSignalStep_UnmarshalJSON_InvalidJSON(t *testing.T) {
	var s SignalStep
	err := s.UnmarshalJSON([]byte(`{invalid`))
	require.Error(t, err)
}

func TestSignalStep_UnmarshalJSON_Success(t *testing.T) {
	data := []byte(`{"type":"signal","title":"stop","signal":"graceful","exit_on_failure":false,"timeout":"5s"}`)
	var s SignalStep
	err := s.UnmarshalJSON(data)
	require.NoError(t, err)
	assert.Equal(t, SignalKindGraceful, s.Signal.Default)
	assert.Equal(t, "5s", s.Timeout.Default)
}

func TestDependenciesStep_Resolve_ReturnsItself(t *testing.T) {
	s := NewDependenciesStep("install deps")

	got := s.Resolve("linux/amd64").(DependenciesStep)

	assert.Equal(t, StepTypeDependencies, got.Type())
	assert.Equal(t, "install deps", got.Title())
}
