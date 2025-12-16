package translator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/fns"
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

	_, err := reader.Arrow("non-existent-file.yaml")
	if err == nil {
		t.Error("TranslateArrow() should return error for non-existent file")
	}
}

func TestReader_TranslateQuiver(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver("non-existent-file.yaml")
	if err == nil {
		t.Error("TranslateQuiver() should return error for non-existent file")
	}
}

func TestReader_ReadManifestInfo(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.ReadSchemaInfo("non-existent-file.yaml")
	if err == nil {
		t.Error("ReadManifestInfo() should return error for non-existent file")
	}
}

func TestReader_ManifestInfo(t *testing.T) {
	reader := NewTranslator()

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.yaml")
	content := []byte("schema: arrow@v1\nmetadata:\n  name: test\n  version: 1.0.0\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	info, err := reader.ReadSchemaInfo(testFile)
	if err != nil {
		t.Errorf("ReadManifestInfo() returned error: %v", err)
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
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid.yaml")
	content := []byte("invalid: yaml: content: [[[")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for invalid YAML")
	}
}

func TestReader_TranslateArrow_MissingSchema(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "no-schema.yaml")
	content := []byte("metadata:\n  name: test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for missing schema")
	}
}

func TestReader_TranslateArrow_WrongSchemaType(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "wrong-type.yaml")
	content := []byte("schema: quiver@v1\nmetadata:\n  name: test\n  version: 1.0.0")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for wrong schema type")
	}
}

func TestReader_TranslateArrow_UnsupportedVersion(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "unsupported.yaml")
	content := []byte("schema: arrow@v999\nmetadata:\n  name: test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for unsupported version")
	}
}

func TestReader_TranslateArrow_InvalidData(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid-data.yaml")
	content := []byte("schema: arrow@v1\nmetadata:\n  name: \"\"\n  version: \"\"")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for invalid data")
	}
}

func TestReader_TranslateQuiver_InvalidYAML(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid.yaml")
	content := []byte("invalid: yaml: [[[")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Quiver(testFile)
	if err == nil {
		t.Error("Quiver() should return error for invalid YAML")
	}
}

func TestReader_TranslateQuiver_WrongSchemaType(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "wrong-type.yaml")
	content := []byte("schema: arrow@v1\nmetadata:\n  name: test\n  version: 1.0.0")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Quiver(testFile)
	if err == nil {
		t.Error("Quiver() should return error for wrong schema type")
	}
}

func TestReader_TranslateQuiver_UnsupportedVersion(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "unsupported.yaml")
	content := []byte("schema: quiver@v999\nmetadata:\n  name: test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Quiver(testFile)
	if err == nil {
		t.Error("Quiver() should return error for unsupported version")
	}
}

func TestReader_ReadSchemaInfo_InvalidYAML(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid.yaml")
	content := []byte("invalid: yaml: [[[")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.ReadSchemaInfo(testFile)
	if err == nil {
		t.Error("ReadSchemaInfo() should return error for invalid YAML")
	}
}

func TestReader_ReadSchemaInfo_MissingSchema(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "no-schema.yaml")
	content := []byte("metadata:\n  name: test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.ReadSchemaInfo(testFile)
	if err == nil {
		t.Error("ReadSchemaInfo() should return error for missing schema")
	}
}

func TestReadFile_Success(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	data, err := fns.Read(context.Background(), testFile)
	if err != nil {
		t.Errorf("readFile() returned error: %v", err)
	}
	if string(data) != "test content" {
		t.Errorf("readFile() = %s, want 'test content'", string(data))
	}
}

func TestReadFile_NonExistent(t *testing.T) {
	_, err := fns.Read(context.Background(), "/non/existent/path.txt")
	if err == nil {
		t.Error("readFile() should return error for non-existent file")
	}
}

func TestReader_TranslateArrow_ValidationFailure(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid-schema.yaml")
	content := []byte(`schema: arrow@v1
metadata:
  name: test-arrow
  version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
  network_mbps: 1
  system:
    - linux/amd64
netbridge:
  - name: 123
    protocol: invalid_protocol
`)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error for validation failure")
	}
}

func TestReader_TranslateArrow_Complete(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "complete-arrow.yaml")
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
  network_mbps: 100
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
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	arrow, err := reader.Arrow(testFile)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Name != "test-arrow" {
		t.Errorf("got name %s, want test-arrow", arrow.Name)
	}
	if arrow.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", arrow.Version)
	}
}

func TestReader_TranslateQuiver_Complete(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "complete-quiver.yaml")
	content := []byte(`manifest: quiver@v1
metadata:
  name: test-quiver
  description: A test quiver
  version: 1.0.0
`)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	quiver, err := reader.Quiver(testFile)
	if err != nil {
		t.Fatalf("Quiver() returned error: %v", err)
	}

	if quiver.Name != "test-quiver" {
		t.Errorf("got name %s, want test-quiver", quiver.Name)
	}
	if quiver.Version != "1.0.0" {
		t.Errorf("got version %s, want 1.0.0", quiver.Version)
	}
}

func TestReader_TranslateQuiver_ValidationFailure(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "invalid-quiver.yaml")
	content := []byte(`manifest: quiver@v1
metadata:
  name: test
`)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Quiver(testFile)
	if err == nil {
		t.Error("Quiver() should return error for validation failure")
	}
}

func TestReader_TranslateArrow_MapperFailure(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "mapper-fail.yaml")
	content := []byte(`schema: arrow@v1
metadata:
  name: ""
  version: ""
`)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := reader.Arrow(testFile)
	if err == nil {
		t.Error("Arrow() should return error when mapper validation fails")
	}
}

func TestReader_TranslateArrow_WithAllActionTypes(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "all-actions.yaml")
	content := []byte(`schema: arrow@v1
metadata:
  name: test-arrow
  description: Test with all action types
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
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
  network_mbps: 1
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
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	arrow, err := reader.Arrow(testFile)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if len(arrow.Methods) == 0 {
		t.Error("Expected at least one method")
	}
	if len(arrow.Methods[0].Actions) != 4 {
		t.Errorf("Expected 4 actions, got %d", len(arrow.Methods[0].Actions))
	}
}

func TestReader_ReadSchemaInfo_WithManifestField(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "manifest-field.yaml")
	content := []byte("manifest: quiver@v1\nmetadata:\n  name: test\n  description: test\n  version: 1.0.0\n")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	info, err := reader.ReadSchemaInfo(testFile)
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
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "complex-nested.yaml")
	content := []byte(`schema: arrow@v1
metadata:
  name: complex-arrow
  description: Arrow with complex nested structures
  version: 1.0.0
  license: Apache-2.0
  quiver: https://github.com/test/complex
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
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
  network_mbps: 1000
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
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	arrow, err := reader.Arrow(testFile)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Name != "complex-arrow" {
		t.Errorf("got name %s, want complex-arrow", arrow.Name)
	}
	if len(arrow.Credits) != 3 {
		t.Errorf("got %d credits, want 3", len(arrow.Credits))
	}
	if len(arrow.Methods) != 3 {
		t.Errorf("got %d methods, want 3", len(arrow.Methods))
	}
}

func TestReader_TranslateQuiver_MinimalValid(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "minimal-quiver.yaml")
	content := []byte(`manifest: quiver@v1
metadata:
  name: q
  description: d
  version: v
`)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	quiver, err := reader.Quiver(testFile)
	if err != nil {
		t.Fatalf("Quiver() returned error: %v", err)
	}

	if quiver.Name != "q" {
		t.Errorf("got name %s, want q", quiver.Name)
	}
}

func TestReader_TranslateArrow_AllFieldsPopulated(t *testing.T) {
	reader := NewTranslator()
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "all-fields.yaml")
	content := []byte(`schema: arrow@v1
metadata:
  name: full-test
  description: Testing all possible fields
  version: 2.5.0
  license: GPL-3.0
  quiver: https://quiver.example.com/repo
  media:
    icon: https://cdn.example.com/icon.png
    banner: https://cdn.example.com/banner.png
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
  network_mbps: 500
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
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	arrow, err := reader.Arrow(testFile)
	if err != nil {
		t.Fatalf("Arrow() returned error: %v", err)
	}

	if arrow.Name != "full-test" {
		t.Errorf("got name %s, want full-test", arrow.Name)
	}
	if arrow.License != "GPL-3.0" {
		t.Errorf("got license %s, want GPL-3.0", arrow.License)
	}
	if len(arrow.Credits) != 2 {
		t.Errorf("got %d credits, want 2", len(arrow.Credits))
	}
	if len(arrow.Netbridge) != 3 {
		t.Errorf("got %d netbridge rules, want 3", len(arrow.Netbridge))
	}
	if len(arrow.Variables) != 3 {
		t.Errorf("got %d variables, want 3", len(arrow.Variables))
	}
	if len(arrow.Methods) != 3 {
		t.Errorf("got %d methods, want 3", len(arrow.Methods))
	}
	if arrow.Requirements.CpuCores != 4 {
		t.Errorf("got cpu_cores %d, want 4", arrow.Requirements.CpuCores)
	}
	if arrow.Requirements.Memory != 8 {
		t.Errorf("got memory %d, want 8", arrow.Requirements.Memory)
	}
}
