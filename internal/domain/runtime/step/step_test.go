package step

import (
	"encoding/json"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewRunStep(t *testing.T) {
	s := NewRunStep("run title", "echo hello", 5*time.Second, true)
	if s.Type() != StepTypeRun {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeRun)
	}
	if s.Title != "run title" {
		t.Errorf("Title = %v, want run title", s.Title)
	}
	if !s.ExitOnFailure {
		t.Error("ExitOnFailure = false, want true")
	}
	if s.Command.Default != "echo hello" {
		t.Errorf("Command.Default = %v, want echo hello", s.Command.Default)
	}
	if s.Timeout.Default != "5s" {
		t.Errorf("Timeout.Default = %v, want 5s", s.Timeout.Default)
	}
}

func TestNewFetchStep(t *testing.T) {
	s := NewFetchStep("fetch title", "https://example.com/file", "/tmp/file", 10*time.Second, false)
	if s.Type() != StepTypeFetch {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeFetch)
	}
	if s.Title != "fetch title" {
		t.Errorf("Title = %v, want fetch title", s.Title)
	}
	if s.ExitOnFailure {
		t.Error("ExitOnFailure = true, want false")
	}
	if s.URL.Default != "https://example.com/file" {
		t.Errorf("URL.Default = %v, want https://example.com/file", s.URL.Default)
	}
	if s.To.Default != "/tmp/file" {
		t.Errorf("To.Default = %v, want /tmp/file", s.To.Default)
	}
}

func TestNewSignalStep(t *testing.T) {
	s := NewSignalStep("signal title", "SIGTERM", 3*time.Second, false)
	if s.Type() != StepTypeSignal {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeSignal)
	}
	if s.Title != "signal title" {
		t.Errorf("Title = %v, want signal title", s.Title)
	}
	if s.Signal.Default != "SIGTERM" {
		t.Errorf("Signal.Default = %v, want SIGTERM", s.Signal.Default)
	}
}

func TestNewDependenciesStep(t *testing.T) {
	s := NewDependenciesStep("deps title")
	if s.Type() != StepTypeDependencies {
		t.Errorf("Type() = %v, want %v", s.Type(), StepTypeDependencies)
	}
	if s.Title != "deps title" {
		t.Errorf("Title = %v, want deps title", s.Title)
	}
}

func TestStepListYAMLUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantLen int
		wantErr bool
		check   func(t *testing.T, list StepList)
	}{
		{
			name: "single run step",
			yaml: `
- type: run
  title: test run
  command: echo hello
  exit_on_failure: false
  timeout: ""
`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				r := list[0].(*RunStep)
				if r.Title != "test run" {
					t.Errorf("Title = %v, want test run", r.Title)
				}
				if r.Type() != StepTypeRun {
					t.Errorf("Type = %v, want run", r.Type())
				}
			},
		},
		{
			name: "mixed steps",
			yaml: `
- type: run
  title: run step
  command: ls
  exit_on_failure: false
  timeout: ""
- type: fetch
  title: fetch step
  url: https://example.com/file
  to: /tmp/file
  exit_on_failure: true
  timeout: 30s
- type: signal
  title: signal step
  signal: SIGTERM
  exit_on_failure: false
  timeout: 5s
- type: dependencies
  title: deps step
`,
			wantLen: 4,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				if len(list) != 4 {
					t.Fatalf("len(list) = %d, want 4", len(list))
				}
				if list[0].Type() != StepTypeRun {
					t.Errorf("step 0 type = %v, want run", list[0].Type())
				}
				if list[1].Type() != StepTypeFetch {
					t.Errorf("step 1 type = %v, want fetch", list[1].Type())
				}
				if list[2].Type() != StepTypeSignal {
					t.Errorf("step 2 type = %v, want signal", list[2].Type())
				}
				if list[3].Type() != StepTypeDependencies {
					t.Errorf("step 3 type = %v, want dependencies", list[3].Type())
				}
			},
		},
		{
			name:    "unknown step type",
			yaml:    `- type: unknown\n  title: test`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name: "not a sequence",
			yaml: `type: run
title: test`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name: "malformed step - unmatched quote",
			yaml: `- type: run
  title: "unclosed
  command: echo
  exit_on_failure: false
  timeout: ""`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name: "missing type field",
			yaml: `- title: test
  command: echo`,
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var list StepList
			err := yaml.Unmarshal([]byte(tt.yaml), &list)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UnmarshalYAML() error = %v, want nil", err)
				return
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
			json:    `[{"type":"run","title":"test","command":"echo hi","exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				r := list[0].(*RunStep)
				if r.Title != "test" {
					t.Errorf("Title = %v, want test", r.Title)
				}
				if r.Command.Default != "echo hi" {
					t.Errorf("Command = %v, want echo hi", r.Command.Default)
				}
			},
		},
		{
			name:    "single fetch step",
			json:    `[{"type":"fetch","title":"fetch test","url":"https://example.com","to":"/tmp","exit_on_failure":true,"timeout":"30s"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				f := list[0].(*FetchStep)
				if f.Title != "fetch test" {
					t.Errorf("Title = %v, want fetch test", f.Title)
				}
				if f.URL.Default != "https://example.com" {
					t.Errorf("URL = %v, want https://example.com", f.URL.Default)
				}
			},
		},
		{
			name:    "mixed step list",
			json:    `[{"type":"run","title":"run","command":"ls","exit_on_failure":false,"timeout":""},{"type":"fetch","title":"fetch","url":"http://test","to":"/home","exit_on_failure":false,"timeout":""},{"type":"signal","title":"signal","signal":"SIGTERM","exit_on_failure":false,"timeout":"5s"},{"type":"dependencies","title":"deps"}]`,
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
			json:    `[{"type":"run"`, // truncated
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "single signal step",
			json:    `[{"type":"signal","title":"sig","signal":"SIGKILL","exit_on_failure":false,"timeout":""}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				s := list[0].(*SignalStep)
				if s.Type() != StepTypeSignal {
					t.Errorf("Type = %v, want signal", s.Type())
				}
			},
		},
		{
			name:    "single dependencies step",
			json:    `[{"type":"dependencies","title":"install"}]`,
			wantLen: 1,
			wantErr: false,
			check: func(t *testing.T, list StepList) {
				d := list[0].(*DependenciesStep)
				if d.Type() != StepTypeDependencies {
					t.Errorf("Type = %v, want dependencies", d.Type())
				}
			},
		},
		{
			name:    "type as number",
			json:    `[{"type":123,"title":"test"}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "type as object",
			json:    `[{"type":{"nested":"obj"},"title":"test"}]`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "type as array",
			json:    `[{"type":[],"title":"test"}]`,
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
				return
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
		o      OverrideableString
		osArch string
		want   string
	}{
		{
			name:   "default only, no override",
			o:      OverrideableString{Default: "base"},
			osArch: "linux/amd64",
			want:   "base",
		},
		{
			name: "default with matching override",
			o: OverrideableString{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64": "linux-build",
					"darwin/arm64": "darwin-build",
				},
			},
			osArch: "linux/amd64",
			want:   "linux-build",
		},
		{
			name: "default with non-matching override",
			o: OverrideableString{
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
			o:      OverrideableString{Default: ""},
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

func TestOverrideableStringYAMLUnmarshal(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantDefault string
		wantOSArch  map[string]string
		wantErr     bool
	}{
		{
			name:        "scalar string",
			yaml:        `"hello"`,
			wantDefault: "hello",
			wantErr:     false,
		},
		{
			name:        "object with default only",
			yaml:        `default: world`,
			wantDefault: "world",
			wantErr:     false,
		},
		{
			name: "object with default and overrides",
			yaml: `default: base
linux/amd64: linux-build
darwin/arm64: darwin-build`,
			wantDefault: "base",
			wantOSArch: map[string]string{
				"linux/amd64": "linux-build",
				"darwin/arm64": "darwin-build",
			},
			wantErr: false,
		},
		{
			name:    "invalid type",
			yaml:    `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name: "only overrides, no default",
			yaml: `linux/amd64: linux-build
darwin/arm64: darwin-build`,
			wantDefault: "",
			wantOSArch: map[string]string{
				"linux/amd64": "linux-build",
				"darwin/arm64": "darwin-build",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o OverrideableString
			err := yaml.Unmarshal([]byte(tt.yaml), &o)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UnmarshalYAML() error = %v, want nil", err)
				return
			}

			if o.Default != tt.wantDefault {
				t.Errorf("Default = %v, want %v", o.Default, tt.wantDefault)
			}

			for k, v := range tt.wantOSArch {
				if o.OSArch[k] != v {
					t.Errorf("OSArch[%s] = %v, want %v", k, o.OSArch[k], v)
				}
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
			wantErr:     false,
		},
		{
			name:        "object with default only",
			json:        `{"default":"world"}`,
			wantDefault: "world",
			wantErr:     false,
		},
		{
			name: "object with default and overrides",
			json: `{"default":"base","linux/amd64":"linux-build","darwin/arm64":"darwin-build"}`,
			wantDefault: "base",
			wantOSArch: map[string]string{
				"linux/amd64": "linux-build",
				"darwin/arm64": "darwin-build",
			},
			wantErr: false,
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
			wantErr:     false,
		},
		{
			name:        "empty object",
			json:        `{}`,
			wantDefault: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o OverrideableString
			err := json.Unmarshal([]byte(tt.json), &o)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("UnmarshalJSON() error = %v, want nil", err)
				return
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
		o    OverrideableString
	}{
		{
			name: "scalar only",
			o:    OverrideableString{Default: "echo hello"},
		},
		{
			name: "with overrides",
			o: OverrideableString{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64":  "linux-build",
					"darwin/arm64": "darwin-build",
				},
			},
		},
		{
			name: "empty default",
			o:    OverrideableString{Default: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.o)
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}

			var got OverrideableString
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

func TestOverrideableStringYAMLRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		o    OverrideableString
	}{
		{
			name: "scalar only",
			o:    OverrideableString{Default: "echo hello"},
		},
		{
			name: "with overrides",
			o: OverrideableString{
				Default: "base",
				OSArch: map[string]string{
					"linux/amd64":  "linux-build",
					"darwin/arm64": "darwin-build",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.o)
			if err != nil {
				t.Fatalf("MarshalYAML() error = %v", err)
			}

			var got OverrideableString
			if err := yaml.Unmarshal(data, &got); err != nil {
				t.Fatalf("UnmarshalYAML() error = %v", err)
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
