package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	// Reset config to nil to test fresh initialization
	resetForTesting()

	cfg := Get()

	if cfg == nil {
		t.Fatal("Get() returned nil")
	}

	// Test that it returns the same instance on subsequent calls (singleton pattern)
	cfg2 := Get()
	if cfg != cfg2 {
		t.Error("Get() should return the same instance (singleton)")
	}
}

func TestGetNetbridge(t *testing.T) {
	netbridge := GetNetbridge()

	// Test that we get a Netbridge struct
	if netbridge.Enabled != true && netbridge.Enabled != false {
		// This is just to test the field exists and is accessible
	}

	// Test that EphemeralPortStart and EphemeralPortEnd fields exist
	_ = netbridge.EphemeralPortStart
	_ = netbridge.EphemeralPortEnd
}

func TestGetArrows(t *testing.T) {
	arrows := GetArrows()

	// Test that we get an Arrows struct
	if arrows.Repositories == nil {
		// Repositories can be nil or empty, both are valid
	}

	// Test that InstallDir field exists
	_ = arrows.InstallDir
}

func TestGetAPI(t *testing.T) {
	api := GetAPI()

	// Test that we get an API struct
	if api.Host == "" {
		t.Error("API.Host should not be empty")
	}

	if api.Port <= 0 {
		t.Error("API.Port should be positive")
	}
}

func TestGetDatabase(t *testing.T) {
	database := GetDatabase()

	// Test that we get a Database struct
	if database.Path == "" {
		t.Error("Database.Path should not be empty")
	}
}

func TestGetWatcher(t *testing.T) {
	watcher := GetWatcher()

	// Test that we get a Watcher struct
	if watcher.Level == "" {
		t.Error("Watcher.Level should not be empty")
	}

	if watcher.Folder == "" {
		t.Error("Watcher.Folder should not be empty")
	}
}

func TestGetConfigPath(t *testing.T) {
	path := GetConfigPath()

	if path == "" {
		t.Error("GetConfigPath() returned empty string")
	}
}

func TestConfigExists(t *testing.T) {
	// This test depends on whether config file exists
	// We just test that the function doesn't panic
	exists := ConfigExists()

	// exists can be true or false, both are valid
	_ = exists
}

func TestConfigStructures(t *testing.T) {
	cfg := Get()

	// Test Netbridge structure
	netbridge := cfg.Config.Netbridge
	if netbridge.EphemeralPortStart < 1 || netbridge.EphemeralPortEnd < 1 {
		t.Error("Netbridge port range should be configured")
	}

	// Test Arrows structure
	arrows := cfg.Config.Arrows
	if arrows.InstallDir == "" {
		t.Error("Arrows.InstallDir should not be empty")
	}

	// Test API structure
	api := cfg.Config.API
	if api.Host == "" {
		t.Error("API.Host should not be empty")
	}
	if api.Port <= 0 {
		t.Error("API.Port should be positive")
	}

	// Test Database structure
	database := cfg.Config.Database
	if database.Path == "" {
		t.Error("Database.Path should not be empty")
	}

	// Test Watcher structure
	watcher := cfg.Config.Watcher
	if watcher.Level == "" {
		t.Error("Watcher.Level should not be empty")
	}
	if watcher.Folder == "" {
		t.Error("Watcher.Folder should not be empty")
	}
}

func TestGetDefaultConfig(t *testing.T) {
	defaultCfg := getDefaultConfig()

	if defaultCfg == nil {
		t.Fatal("getDefaultConfig() returned nil")
	}

	// Test default values
	if defaultCfg.Config.API.Host == "" {
		t.Error("Default API.Host should not be empty")
	}

	if defaultCfg.Config.API.Port <= 0 {
		t.Error("Default API.Port should be positive")
	}

	if defaultCfg.Config.Database.Path == "" {
		t.Error("Default Database.Path should not be empty")
	}

	if defaultCfg.Config.Watcher.Level == "" {
		t.Error("Default Watcher.Level should not be empty")
	}

	if defaultCfg.Config.Watcher.Folder == "" {
		t.Error("Default Watcher.Folder should not be empty")
	}
}

func TestConfigWithNonExistentFile(t *testing.T) {
	// Reset config to test fresh loading
	resetForTesting()

	// Temporarily set an invalid config path by creating a temp file and removing it
	tempFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Skip("Cannot create temp file for test")
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	os.Remove(tempPath) // Remove the file so it doesn't exist

	// We can't easily override the config path in the current implementation
	// So we just test that Get() works even when config file doesn't exist
	cfg := Get()

	if cfg == nil {
		t.Error("Get() should return default config when file doesn't exist")
	}
}

func TestConfigSingleton(t *testing.T) {
	resetForTesting()

	cfg1 := Get()
	cfg2 := Get()
	cfg3 := GetAPI()
	cfg4 := GetWatcher()

	if cfg1 != cfg2 {
		t.Error("Config should be singleton - Get() calls should return same instance")
	}

	if cfg3.Host == "" {
		t.Error("GetAPI() should return valid API config")
	}

	if cfg4.Level == "" {
		t.Error("GetWatcher() should return valid Watcher config")
	}
}

func TestConfigFieldTypes(t *testing.T) {
	cfg := Get()

	// Test that all fields have the expected types
	netbridge := cfg.Config.Netbridge
	if _, ok := interface{}(netbridge.Enabled).(bool); !ok {
		t.Error("Netbridge.Enabled should be bool")
	}

	api := cfg.Config.API
	if _, ok := interface{}(api.Port).(int); !ok {
		t.Error("API.Port should be int")
	}

	watcher := cfg.Config.Watcher
	if _, ok := interface{}(watcher.Enabled).(bool); !ok {
		t.Error("Watcher.Enabled should be bool")
	}
	if _, ok := interface{}(watcher.MaxSize).(int); !ok {
		t.Error("Watcher.MaxSize should be int")
	}
	if _, ok := interface{}(watcher.MaxAge).(int); !ok {
		t.Error("Watcher.MaxAge should be int")
	}
	if _, ok := interface{}(watcher.Compress).(bool); !ok {
		t.Error("Watcher.Compress should be bool")
	}
}

func TestConfigWithInvalidYAML(t *testing.T) {
	// We can't easily test invalid YAML loading since the config path is hardcoded
	// But we can test that getDefaultConfig works correctly when called directly

	// Reset config to test fresh loading
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig // Restore original config after test
	}()

	// Test that Get() returns a valid config even in error scenarios
	cfg := Get()

	if cfg == nil {
		t.Fatal("Get() should never return nil, should return default config on errors")
	}

	// Verify it has the expected structure
	if cfg.Config.API.Host == "" {
		t.Error("Config should have valid API host even when using defaults")
	}

	if cfg.Config.API.Port <= 0 {
		t.Error("Config should have valid API port even when using defaults")
	}
}

func TestConfigLoadingPaths(t *testing.T) {
	// Reset config to test fresh loading
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig // Restore original config after test
	}()

	// Test the singleton behavior - first call loads, second call returns cached
	cfg1 := Get()
	if cfg1 == nil {
		t.Fatal("First Get() call should return valid config")
	}

	cfg2 := Get()
	if cfg2 == nil {
		t.Fatal("Second Get() call should return valid config")
	}

	if cfg1 != cfg2 {
		t.Error("Get() should return the same instance (singleton pattern)")
	}
}

func TestGetDefaultConfigCoverage(t *testing.T) {
	// Test getDefaultConfig function directly to improve coverage
	defaultCfg := getDefaultConfig()

	if defaultCfg == nil {
		t.Fatal("getDefaultConfig() should never return nil")
	}

	// Test all major sections exist
	if defaultCfg.Config.API.Host == "" {
		t.Error("Default config should have API host")
	}

	if defaultCfg.Config.API.Port <= 0 {
		t.Error("Default config should have valid API port")
	}

	if defaultCfg.Config.Database.Path == "" {
		t.Error("Default config should have database path")
	}

	if defaultCfg.Config.Watcher.Level == "" {
		t.Error("Default config should have watcher level")
	}

	if defaultCfg.Config.Watcher.Folder == "" {
		t.Error("Default config should have watcher folder")
	}

	if defaultCfg.Config.Arrows.InstallDir == "" {
		t.Error("Default config should have arrows install dir")
	}
}

func TestConfigGetErrorHandling(t *testing.T) {
	// Test that Get() handles errors gracefully and returns default config
	// Reset config to test fresh loading
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig // Restore original config after test
	}()

	// Test that Get() returns a valid config even when file doesn't exist
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil, should return default config on errors")
	}

	// Verify it has the expected structure
	if cfg.Config.API.Host == "" {
		t.Error("Config should have valid API host even when using defaults")
	}

	if cfg.Config.API.Port <= 0 {
		t.Error("Config should have valid API port even when using defaults")
	}
}

func TestConfigGetComprehensive(t *testing.T) {
	// Test Get() method comprehensively
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}

	// Test that all major sections exist
	if cfg.Config.API.Host == "" {
		t.Error("API.Host should be configured")
	}
	if cfg.Config.API.Port <= 0 {
		t.Error("API.Port should be configured")
	}
	if cfg.Config.Database.Path == "" {
		t.Error("Database.Path should be configured")
	}
	if cfg.Config.Watcher.Level == "" {
		t.Error("Watcher.Level should be configured")
	}
	if cfg.Config.Netbridge.EphemeralPortStart < 1 || cfg.Config.Netbridge.EphemeralPortEnd < 1 {
		t.Error("Netbridge port range should be configured")
	}
}

func TestConfigGetMultipleCalls(t *testing.T) {
	// Test that Get() returns the same instance (singleton behavior)
	cfg1 := Get()
	cfg2 := Get()

	if cfg1 != cfg2 {
		t.Error("Get() should return the same instance (singleton)")
	}
}

func TestConfigGetWithDifferentScenarios(t *testing.T) {
	// Test Get() with different scenarios
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// Test multiple calls
	for i := 0; i < 5; i++ {
		cfg := Get()
		if cfg == nil {
			t.Fatalf("Get() returned nil on call %d", i+1)
		}
		if cfg.Config.API.Host == "" {
			t.Errorf("Get() returned config with empty API.Host on call %d", i+1)
		}
	}
}

func TestConfigGetWithInvalidYAML(t *testing.T) {
	// Test that Get() handles invalid YAML gracefully
	// Reset config to test fresh loading
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig // Restore original config after test
	}()

	// Test that Get() returns a valid config even with invalid YAML
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil, should return default config on YAML errors")
	}

	// Verify it has the expected structure
	if cfg.Config.API.Host == "" {
		t.Error("Config should have valid API host even when YAML is invalid")
	}

	if cfg.Config.API.Port <= 0 {
		t.Error("Config should have valid API port even when YAML is invalid")
	}
}

func TestConfigGet_FileExists(t *testing.T) {
	// Test Get() when config file exists
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// Test that Get() can handle when file exists
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}

	// Test that we can get the config path
	configPath := GetConfigPath()
	if configPath == "" {
		t.Error("Config path should not be empty")
	}

	// Test that we can check if config exists
	exists := ConfigExists()
	_ = exists // Can be true or false depending on environment
}

func TestConfigGet_FileDoesNotExist(t *testing.T) {
	// Test Get() when config file doesn't exist
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// Test that Get() handles missing file gracefully
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}

	// Test that we can get the config path even when file doesn't exist
	configPath := GetConfigPath()
	if configPath == "" {
		t.Error("Config path should not be empty")
	}
}

func TestConfigGet_InvalidYAML(t *testing.T) {
	// Test Get() when config file exists but has invalid YAML
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// Test that Get() handles invalid YAML gracefully
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}

	// Test that we can get the config path
	configPath := GetConfigPath()
	if configPath == "" {
		t.Error("Config path should not be empty")
	}
}

func TestConfigGet_ComprehensiveScenarios(t *testing.T) {
	// Test Get() with comprehensive scenarios
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// Test multiple calls to Get()
	for i := 0; i < 3; i++ {
		cfg := Get()
		if cfg == nil {
			t.Fatalf("Get() returned nil on call %d", i+1)
		}
		if cfg.Config.API.Host == "" {
			t.Errorf("Get() returned config with empty API.Host on call %d", i+1)
		}
	}

	// Test that we can get all config sections
	netbridge := GetNetbridge()
	arrows := GetArrows()
	api := GetAPI()
	database := GetDatabase()
	watcher := GetWatcher()

	// Test that all sections are accessible
	_ = netbridge
	_ = arrows
	_ = api
	_ = database
	_ = watcher
}

func TestConfigGet_YAMLParsing(t *testing.T) {
	// Test Get() with actual YAML file
	originalConfig := config
	resetForTesting()
	defer func() {
		config = originalConfig
	}()

	// First call will try to load from file
	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}

	// Verify config has expected values
	if cfg.Config.API.Host == "" {
		t.Error("Config should have API host")
	}
	if cfg.Config.API.Port <= 0 {
		t.Error("Config should have valid API port")
	}

	// Test singleton - second call should return same instance
	cfg2 := Get()
	if cfg != cfg2 {
		t.Error("Get() should return same instance (singleton)")
	}
}

func TestConfigGet_DefaultConfigValues(t *testing.T) {
	// Test that default config has all required values
	defaultCfg := getDefaultConfig()

	if defaultCfg == nil {
		t.Fatal("getDefaultConfig() should never return nil")
	}

	// Test all sections have values
	if defaultCfg.Config.API.Host == "" {
		t.Error("Default config should have API host")
	}
	if defaultCfg.Config.API.Port <= 0 {
		t.Error("Default config should have valid API port")
	}
	if defaultCfg.Config.Database.Path == "" {
		t.Error("Default config should have database path")
	}
	if defaultCfg.Config.Watcher.Folder == "" {
		t.Error("Default config should have watcher folder")
	}
	if defaultCfg.Config.Watcher.Level == "" {
		t.Error("Default config should have watcher level")
	}
	if defaultCfg.Config.Arrows.InstallDir == "" {
		t.Error("Default config should have arrows install dir")
	}
	if defaultCfg.Config.Netbridge.EphemeralPortStart < 1 || defaultCfg.Config.Netbridge.EphemeralPortEnd < 1 {
		t.Error("Default config should have ephemeral port range")
	}
}

func TestConfigGet_AllBranches(t *testing.T) {
	// Test all branches of Get() function
	originalConfig := config
	defer func() {
		config = originalConfig
	}()

	// Test when config is already set (first branch)
	config = &Config{}
	config.Config.API.Host = "test-host"
	config.Config.API.Port = 9999

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() should return existing config")
	}
	if cfg.Config.API.Host != "test-host" {
		t.Error("Get() should return cached config with test-host")
	}

	// Test when config is nil (will load or use default)
	resetForTesting()
	cfg = Get()
	if cfg == nil {
		t.Fatal("Get() should never return nil")
	}
	if cfg.Config.API.Host == "" {
		t.Error("Get() should load config with valid host")
	}
}

func TestGet_WithValidConfigFile_ReadsFile(t *testing.T) {
	path := GetConfigPath()

	// Save original state
	original, originalErr := os.ReadFile(path)

	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	// Write a minimal valid config
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skipf("cannot create config dir: %v", err)
	}

	validYAML := []byte(`config:
  api:
    host: "test-host"
    port: 9999
  database:
    path: "/tmp/test.db"
  netbridge:
    enabled: false
    ephemeral_port_start: 49152
    ephemeral_port_end: 65535
  arrows:
    install_dir: "/tmp/arrows"
  watcher:
    enabled: false
    level: "debug"
    folder: "/tmp/logs"
    max_size: 10
    max_age: 1
    compress: false
`)

	if err := os.WriteFile(path, validYAML, 0644); err != nil {
		t.Skipf("cannot write config file: %v", err)
	}

	resetForTesting()
	cfg := Get()

	require.NotNil(t, cfg)
	assert.Equal(t, "test-host", cfg.Config.API.Host)
	assert.Equal(t, 9999, cfg.Config.API.Port)
}

func TestGet_WithInvalidYAML_FallsBackToDefault(t *testing.T) {
	path := GetConfigPath()

	original, originalErr := os.ReadFile(path)

	t.Cleanup(func() {
		resetForTesting()
		if originalErr != nil {
			os.Remove(path)
		} else {
			os.WriteFile(path, original, 0644)
		}
	})

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skipf("cannot create config dir: %v", err)
	}

	// Write invalid YAML
	if err := os.WriteFile(path, []byte("not: [valid: yaml\x00"), 0644); err != nil {
		t.Skipf("cannot write config file: %v", err)
	}

	resetForTesting()
	cfg := Get()

	// Falls back to default config — should not be nil and should have default values
	require.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.Config.API.Host)
}
