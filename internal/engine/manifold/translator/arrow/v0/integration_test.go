package v0_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

const gameServerYAML = `
schema: "arrow@v0"
metadata:
  name: char2cs.myserver
  description: My awesome Linux game server
  license: MIT
  maintainers:
    - name: char2cs
      email: me@char2cs.net
netbridge:
  - name: GAME_PORT
    default: 27015
    protocol: tcp/udp
    required: true
targets:
  "linux/*":
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 20
    lifecycle:
      install:
        - type: fetch
          url:
            linux/amd64: https://releases.myserver.io/v1.0.0/myserver-linux-amd64.tar.gz
            linux/arm64: https://releases.myserver.io/v1.0.0/myserver-linux-arm64.tar.gz
          to: ./myserver.tar.gz
          title: Downloading server binary
          timeout: 10m
          exit_on_failure: true
        - type: run
          command: tar -xzf ./myserver.tar.gz
          title: Extracting server
          timeout: 5m
          exit_on_failure: true
      execute:
        - type: run
          command: ./myserver --port ${GAME_PORT}
          title: Starting game server
          timeout: 30s
          exit_on_failure: true
      stop:
        - type: signal
          signal: graceful
          timeout: 30s
          exit_on_failure: false
      uninstall:
        - type: run
          command: rm -f ./myserver ./myserver.tar.gz
          title: Removing server binary
          timeout: 30s
          exit_on_failure: false
`

func TestIntegration_LinuxGameServer(t *testing.T) {
	manifest, precompiled, err := v0.New().Parse([]byte(gameServerYAML))
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if err := compiler.New().Compile(manifest, precompiled, v0.New().Selector()); err != nil {
		t.Fatalf("compiler.Compile() error = %v", err)
	}
	if err := ruleset.New().ValidateCompiled(manifest); err != nil {
		t.Fatalf("ValidateCompiled() error = %v", err)
	}

	amd64, ok := manifest.Targets[domain.OSLinuxAMD64]
	if !ok {
		t.Fatal("linux/amd64 not in compiled targets")
	}
	if len(amd64.Lifecycle.Install) == 0 {
		t.Fatal("expected at least one install step for linux/amd64")
	}
	fetchAMD64, ok := amd64.Lifecycle.Install[0].(step.FetchStep)
	if !ok {
		t.Fatalf("install[0] is %T, want FetchStep", amd64.Lifecycle.Install[0])
	}
	wantAMD64URL := "https://releases.myserver.io/v1.0.0/myserver-linux-amd64.tar.gz"
	if fetchAMD64.URL.Default != wantAMD64URL {
		t.Errorf("linux/amd64 URL.Default = %q, want %q", fetchAMD64.URL.Default, wantAMD64URL)
	}

	arm64, ok := manifest.Targets[domain.OSLinuxARM64]
	if !ok {
		t.Fatal("linux/arm64 not in compiled targets")
	}
	if len(arm64.Lifecycle.Install) == 0 {
		t.Fatal("expected at least one install step for linux/arm64")
	}
	fetchARM64, ok := arm64.Lifecycle.Install[0].(step.FetchStep)
	if !ok {
		t.Fatalf("install[0] is %T, want FetchStep", arm64.Lifecycle.Install[0])
	}
	wantARM64URL := "https://releases.myserver.io/v1.0.0/myserver-linux-arm64.tar.gz"
	if fetchARM64.URL.Default != wantARM64URL {
		t.Errorf("linux/arm64 URL.Default = %q, want %q", fetchARM64.URL.Default, wantARM64URL)
	}

	_, okDarwin := manifest.Targets[domain.OSDarwinAMD64]
	if okDarwin {
		t.Error("darwin/amd64 should not be in compiled targets (no matching target in manifest)")
	}

	// Also verify SelectTarget directly on the precompiled map
	_, errDarwin := v0.SelectTarget(precompiled, domain.OSDarwinAMD64)
	if !errors.Is(errDarwin, models.ErrNoTargetForOS) {
		t.Errorf("SelectTarget(darwin/amd64) error = %v, want ErrNoTargetForOS", errDarwin)
	}
}

func TestV0Translator_DemoArrows(t *testing.T) {
	entries, err := filepath.Glob("../../../../../../docs/templates/demo/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, entries, "no demo arrows found")

	for _, path := range entries {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			_, _, err = v0.Map(data)
			assert.NoError(t, err, "demo arrow must parse without error")
		})
	}
}
