package translator

import (
	"testing"
)

func TestNewReader(t *testing.T) {
	reader := NewTranslator()
	if reader == nil {
		t.Fatal("NewReader() returned nil")
	}
}

func TestReader_MultipleInstances(t *testing.T) {
	reader1 := NewTranslator()
	reader2 := NewTranslator()

	if reader1 == nil || reader2 == nil {
		t.Error("NewReader() returned nil instance")
	}
}

func TestReader_TranslateArrow(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("Arrow() should return error for invalid input")
	}
}

func TestReader_TranslateQuiver(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("Quiver() should return error for invalid input")
	}
}

func TestReader_ReadManifestInfo(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.ReadSchemaInfo([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("ReadSchemaInfo() should return error for invalid input")
	}
}

func TestReader_ManifestInfo(t *testing.T) {
	reader := NewTranslator()
	content := []byte("schema: arrow@v1\nmetadata:\n  name: test\n  version: 1.0.0\n")

	info, err := reader.ReadSchemaInfo(content)
	if err != nil {
		t.Errorf("ReadSchemaInfo() returned error: %v", err)
	}

	if info.SchemaType != "arrow" {
		t.Errorf("Expected SchemaType 'arrow', got '%s'", info.SchemaType)
	}

	if info.Version != "v1" {
		t.Errorf("Expected Version 'v1', got '%s'", info.Version)
	}

	if info.ManifestKey != "arrow@v1" {
		t.Errorf("Expected ManifestKey 'arrow@v1', got '%s'", info.ManifestKey)
	}
}

func TestReader_TranslateArrow_InvalidYAML(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("invalid: yaml: content: [[["))
	if err == nil {
		t.Error("Arrow() should return error for invalid YAML")
	}
}

func TestReader_TranslateArrow_MissingSchema(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("metadata:\n  name: test"))
	if err == nil {
		t.Error("Arrow() should return error for missing schema")
	}
}

func TestReader_TranslateArrow_WrongSchemaType(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("schema: quiver@v1\nmetadata:\n  name: test\n  version: 1.0.0"))
	if err == nil {
		t.Error("Arrow() should return error for wrong schema type")
	}
}

func TestReader_TranslateArrow_UnsupportedVersion(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("schema: arrow@v999\nmetadata:\n  name: test"))
	if err == nil {
		t.Error("Arrow() should return error for unsupported version")
	}
}

func TestReader_TranslateArrow_InvalidData(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("schema: arrow@v1\nmetadata:\n  name: \"\"\n  version: \"\""))
	if err == nil {
		t.Error("Arrow() should return error for invalid data")
	}
}

func TestReader_TranslateQuiver_InvalidYAML(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("Quiver() should return error for invalid YAML")
	}
}

func TestReader_TranslateQuiver_WrongSchemaType(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver([]byte("schema: arrow@v1\nmetadata:\n  name: test\n  version: 1.0.0"))
	if err == nil {
		t.Error("Quiver() should return error for wrong schema type")
	}
}

func TestReader_TranslateQuiver_UnsupportedVersion(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver([]byte("schema: quiver@v999\nmetadata:\n  name: test"))
	if err == nil {
		t.Error("Quiver() should return error for unsupported version")
	}
}

func TestReader_ReadSchemaInfo_InvalidYAML(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.ReadSchemaInfo([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("ReadSchemaInfo() should return error for invalid YAML")
	}
}

func TestReader_ReadSchemaInfo_MissingSchema(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.ReadSchemaInfo([]byte("metadata:\n  name: test"))
	if err == nil {
		t.Error("ReadSchemaInfo() should return error for missing schema")
	}
}

func TestReader_TranslateArrow_ValidationFailure(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: test-arrow
  version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
  system:
    - linux/amd64
netbridge:
  - name: 123
    protocol: invalid_protocol
`)

	_, err := reader.Arrow(content)
	if err == nil {
		t.Error("Arrow() should return error for validation failure")
	}
}

func TestReader_TranslateArrow_Complete(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: test-arrow
  description: A test arrow
  version: 1.0.0
  license: MIT
  quiver: https://github.com/test/test
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
  credits:
    - name: Test User
      email: test@example.com
      url: https://example.com
requirements:
  cpu_cores: 2
  ram_gb: 4
  disk_gb: 10
netbridge:
  - name: TEST_PORT
    protocol: tcp
variables:
  - name: TEST_VAR
    default: value1
    sensitive: false
wizards:
  - platforms:
      - linux/amd64
    workdir: /app
    methods:
      - method: install
        actions:
          - name: Installing
            run: echo install
`)

	arrow, err := reader.Arrow(content)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Manifest.Name != "test-arrow" {
		t.Errorf("got name %s, want test-arrow", arrow.Manifest.Name)
	}
	if arrow.Manifest.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", arrow.Manifest.Version)
	}
}

func TestReader_TranslateQuiver_Complete(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`manifest: quiver@v1
metadata:
  name: test-quiver
  description: A test quiver
  version: 1.0.0
`)

	quiver, err := reader.Quiver(content)
	if err != nil {
		t.Fatalf("Quiver() returned error: %v", err)
	}

	if quiver.Manifest.Name != "test-quiver" {
		t.Errorf("got name %s, want test-quiver", quiver.Manifest.Name)
	}
}

func TestReader_TranslateQuiver_ValidationFailure(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`manifest: quiver@v1
metadata:
  name: test
`)

	_, err := reader.Quiver(content)
	if err == nil {
		t.Error("Quiver() should return error for validation failure")
	}
}

func TestReader_TranslateArrow_MapperFailure(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: ""
  version: ""
`)

	_, err := reader.Arrow(content)
	if err == nil {
		t.Error("Arrow() should return error when mapper validation fails")
	}
}

func TestReader_TranslateArrow_WithAllActionTypes(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: test-arrow
  description: Test with all action types
  version: 1.0.0
  license: MIT
  quiver: https://github.com/test/test
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
netbridge:
  - name: HTTP
    protocol: tcp
  - name: UDP_PORT
    protocol: udp
  - name: BOTH
    protocol: tcp/udp
variables:
  - name: VAR1
    default: default_value
    sensitive: true
wizards:
  - platforms:
      - linux/amd64
      - windows/amd64
    workdir: /opt
    methods:
      - method: install
        actions:
          - name: Download
            download: https://example.com/file.sh
            to: /tmp/file.sh
          - name: Copy
            copy: source.txt
            to: dest.txt
          - name: Uncompress
            uncompress: archive.tar.gz
            to: /opt/extracted
          - name: Run
            run: echo "Done"
            exit_on_failure: true
            timeout: 60s
`)

	arrow, err := reader.Arrow(content)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if len(arrow.Manifest.Lifecycle.Install) == 0 {
		t.Error("Expected at least one install lifecycle step")
	}
	if len(arrow.Manifest.Lifecycle.Install) != 4 {
		t.Errorf("Expected 4 install steps, got %d", len(arrow.Manifest.Lifecycle.Install))
	}
}

func TestReader_ReadSchemaInfo_WithManifestField(t *testing.T) {
	reader := NewTranslator()
	content := []byte("manifest: quiver@v1\nmetadata:\n  name: test\n  description: test\n  version: 1.0.0\n")

	info, err := reader.ReadSchemaInfo(content)
	if err != nil {
		t.Errorf("ReadSchemaInfo() returned error: %v", err)
	}

	if info.SchemaType != "quiver" {
		t.Errorf("Expected SchemaType 'quiver', got '%s'", info.SchemaType)
	}

	if info.Version != "v1" {
		t.Errorf("Expected Version 'v1', got '%s'", info.Version)
	}

	if info.ManifestKey != "quiver@v1" {
		t.Errorf("Expected ManifestKey 'quiver@v1', got '%s'", info.ManifestKey)
	}
}

func TestReader_TranslateArrow_ComplexNestedStructure(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: complex-arrow
  description: Arrow with complex nested structures
  version: 1.0.0
  license: Apache-2.0
  quiver: https://github.com/test/complex
  credits:
    - name: Developer One
      email: dev1@example.com
      url: https://example.com/dev1
    - name: Developer Two
      email: dev2@example.com
      url: https://example.com/dev2
    - name: Developer Three
      email: dev3@example.com
      url: https://example.com/dev3
requirements:
  cpu_cores: 8
  ram_gb: 16
  disk_gb: 100
netbridge:
  - name: WEB_HTTP
    protocol: tcp
  - name: WEB_HTTPS
    protocol: tcp
  - name: GAME_UDP
    protocol: udp
  - name: COMBINED
    protocol: tcp/udp
variables:
  - name: SERVER_PORT
    default: "8080"
    sensitive: false
  - name: API_KEY
    default: "default-key"
    sensitive: true
  - name: MAX_CONNECTIONS
    default: "100"
    sensitive: false
wizards:
  - platforms:
      - linux/amd64
      - linux/arm64
    workdir: /opt/app
    methods:
      - method: install
        actions:
          - name: Download binary
            download: https://example.com/app.sh
            to: /tmp/app.sh
          - name: Execute installation
            run: bash /tmp/app.sh
            exit_on_failure: true
            timeout: 300s
      - method: update
        actions:
          - name: Stop service
            run: systemctl stop app
          - name: Download update
            download: https://example.com/update.sh
            to: /tmp/update.sh
          - name: Apply update
            run: bash /tmp/update.sh
            exit_on_failure: true
  - platforms:
      - windows/amd64
    workdir: C:\Program Files\App
    methods:
      - method: install
        actions:
          - name: Download installer
            download: https://example.com/installer.zip
            to: C:\Temp\installer.zip
          - name: Uncompress installer
            uncompress: C:\Temp\installer.zip
            to: C:\Temp\installer
          - name: Run installer
            run: C:\Temp\installer\setup.exe
            exit_on_failure: true
`)

	arrow, err := reader.Arrow(content)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Manifest.Name != "complex-arrow" {
		t.Errorf("got name %s, want complex-arrow", arrow.Manifest.Name)
	}
	if len(arrow.Manifest.Credits) != 3 {
		t.Errorf("got %d credits, want 3", len(arrow.Manifest.Credits))
	}
	// "install" goes to Lifecycle.Install (2+3=5 steps), "update" goes to Methods map
	if len(arrow.Manifest.Lifecycle.Install) == 0 {
		t.Error("Expected install lifecycle steps")
	}
	if _, ok := arrow.Manifest.Methods["update"]; !ok {
		t.Error("Expected update method in Methods map")
	}
}

func TestReader_TranslateQuiver_MinimalValid(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`manifest: quiver@v1
metadata:
  name: q
  description: d
  version: v
`)

	quiver, err := reader.Quiver(content)
	if err != nil {
		t.Fatalf("Quiver() returned error: %v", err)
	}

	if quiver.Manifest.Name != "q" {
		t.Errorf("got name %s, want q", quiver.Manifest.Name)
	}
}

func TestReader_TranslateArrow_AllFieldsPopulated(t *testing.T) {
	reader := NewTranslator()
	content := []byte(`schema: arrow@v1
metadata:
  name: full-test
  description: Testing all possible fields
  version: 2.5.0
  license: GPL-3.0
  quiver: https://quiver.example.com/repo
  credits:
    - name: Alice
      email: alice@example.com
      url: https://alice.example.com
    - name: Bob
      email: bob@example.com
      url: https://bob.example.com
requirements:
  cpu_cores: 4
  ram_gb: 8
  disk_gb: 50
netbridge:
  - name: PRIMARY_TCP
    protocol: tcp
  - name: SECONDARY_UDP
    protocol: udp
  - name: FALLBACK_BOTH
    protocol: tcp/udp
variables:
  - name: CONFIG_PATH
    default: /etc/app/config
    sensitive: false
  - name: SECRET_TOKEN
    default: changeme
    sensitive: true
  - name: PORT_NUMBER
    default: "3000"
    sensitive: false
    values:
      - "3000"
      - "3001"
      - "3002"
    min: 3000
    max: 4000
wizards:
  - platforms:
      - linux/amd64
      - linux/arm64
      - darwin/amd64
      - darwin/arm64
    dependencies:
      - git
      - curl
      - tar
    workdir: /opt/application
    methods:
      - method: install
        actions:
          - name: Create directory
            run: mkdir -p /opt/application
          - name: Download archive
            download: https://releases.example.com/app.tar.gz
            to: /tmp/app.tar.gz
          - name: Extract files
            uncompress: /tmp/app.tar.gz
            to: /opt/application
          - name: Copy config
            copy: config.template
            to: /etc/app/config
          - name: Start service
            run: systemctl start application
            exit_on_failure: true
            timeout: 120s
      - method: uninstall
        actions:
          - name: Stop service
            run: systemctl stop application
          - name: Remove files
            run: rm -rf /opt/application
      - method: restart
        actions:
          - name: Restart service
            run: systemctl restart application
            exit_on_failure: false
`)

	arrow, err := reader.Arrow(content)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Manifest.Name != "full-test" {
		t.Errorf("got name %s, want full-test", arrow.Manifest.Name)
	}
	if arrow.Manifest.License != "GPL-3.0" {
		t.Errorf("got license %s, want GPL-3.0", arrow.Manifest.License)
	}
	if len(arrow.Manifest.Credits) != 2 {
		t.Errorf("got %d credits, want 2", len(arrow.Manifest.Credits))
	}
	if len(arrow.Manifest.Netbridge) != 3 {
		t.Errorf("got %d netbridge rules, want 3", len(arrow.Manifest.Netbridge))
	}
	if len(arrow.Manifest.Variables) != 3 {
		t.Errorf("got %d variables, want 3", len(arrow.Manifest.Variables))
	}
	if arrow.Manifest.Requirements.CpuCores != 4 {
		t.Errorf("got cpu_cores %d, want 4", arrow.Manifest.Requirements.CpuCores)
	}
	if arrow.Manifest.Requirements.MemoryGB != 8 {
		t.Errorf("got MemoryGB %d, want 8", arrow.Manifest.Requirements.MemoryGB)
	}
	// install and uninstall go to lifecycle; restart goes to Methods map
	if len(arrow.Manifest.Lifecycle.Install) == 0 {
		t.Error("Expected install lifecycle steps")
	}
	if len(arrow.Manifest.Lifecycle.Uninstall) == 0 {
		t.Error("Expected uninstall lifecycle steps")
	}
	if _, ok := arrow.Manifest.Methods["restart"]; !ok {
		t.Error("Expected restart method in Methods map")
	}
}
