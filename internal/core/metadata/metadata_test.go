package metadata

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_ReturnsSingleton(t *testing.T) {
	assert.Same(t, Get(), Get())
}

func TestGetVersion_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetVersion())
}

func TestGetVersionCodename_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetVersionCodename())
}

func TestGetName_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetName())
}

func TestGetDescription_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetDescription())
}

func TestGetAuthor_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetAuthor())
}

func TestGetURL_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetURL())
}

func TestGetLicense_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetLicense())
}

func TestGetCopyright_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetCopyright())
}

func TestMetadataStructure_AllFieldsPopulated(t *testing.T) {
	m := Get()
	assert.NotEmpty(t, m.Version.Number)
	assert.NotEmpty(t, m.Version.Codename)
	assert.NotEmpty(t, m.Metadata.Name)
	assert.NotEmpty(t, m.Metadata.Description)
	assert.NotEmpty(t, m.Metadata.Author)
	assert.NotEmpty(t, m.Metadata.URL)
	assert.NotEmpty(t, m.Metadata.License)
	assert.NotEmpty(t, m.Metadata.Copyright)
}

func TestDefaultMetadata_NonNil(t *testing.T) {
	require.NotNil(t, defaultMetadata())
}

func TestDefaultMetadata_PathsPopulated(t *testing.T) {
	d := defaultMetadata()
	assert.NotEmpty(t, d.Paths.Home.Default)
	assert.NotEmpty(t, d.Paths.Events)
	assert.NotEmpty(t, d.Paths.Store)
	assert.NotEmpty(t, d.Paths.Namespaces)
	assert.NotEmpty(t, d.Paths.Config)
	assert.NotEmpty(t, d.Paths.Logs)
	assert.NotEmpty(t, d.Paths.Vault)
}

func TestGetHomePath_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, GetHomePath())
}

func TestGetHomePath_EndsWithQuiver(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetHomePath(), ".quiver"),
		"expected path to end in .quiver, got %q", GetHomePath())
}

func TestGetHomePath_Unix_AbsoluteUnderUserHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix test")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".quiver"), GetHomePath())
}

func TestGetEventsPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetEventsPath(), GetHomePath()))
}

func TestGetEventsPath_EndsWithStateEvents(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetEventsPath(), filepath.Join("state", "events")))
}

func TestGetStorePath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetStorePath(), GetHomePath()))
}

func TestGetStorePath_EndsWithStateStore(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetStorePath(), filepath.Join("state", "store")))
}

func TestGetNamespacesPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetNamespacesPath(), GetHomePath()))
}

func TestGetNamespacesPath_EndsWithNamespaces(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetNamespacesPath(), "namespaces"))
}

func TestGetConfigPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetConfigPath(), GetHomePath()))
}

func TestGetConfigPath_EndsWithConfigYaml(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetConfigPath(), "config.yaml"))
}

func TestGetLogsPath_ContainsHome(t *testing.T) {
	assert.True(t, strings.HasPrefix(GetLogsPath(), GetHomePath()))
}

func TestGetLogsPath_EndsWithLogs(t *testing.T) {
	assert.True(t, strings.HasSuffix(GetLogsPath(), "logs"))
}

func TestGetVaultPath(t *testing.T) {
	path := GetVaultPath()
	assert.NotEmpty(t, path)
	// path should end in /vault (or \vault on Windows)
	assert.True(t,
		strings.HasSuffix(path, "/vault") || strings.HasSuffix(path, `\vault`),
		"expected path to end in /vault, got: %s", path,
	)
}

func TestGetPlatforms_ReturnsKnownDomains(t *testing.T) {
	platforms := GetPlatforms()
	require.NotNil(t, platforms)
	for _, domain := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
		assert.Contains(t, platforms, domain, "expected %q in platforms", domain)
	}
}

func TestGetPlatforms_GitHubRawURL(t *testing.T) {
	github := GetPlatforms()["github.com"]
	assert.Contains(t, github.RawURL, "raw.githubusercontent.com")
	assert.Equal(t, "main", github.DefaultBranch)
}

func TestMetadataConsistency(t *testing.T) {
	m := Get()
	assert.Equal(t, GetVersion(), m.Version.Number)
	assert.Equal(t, GetVersionCodename(), m.Version.Codename)
	assert.Equal(t, GetName(), m.Metadata.Name)
	assert.Equal(t, GetDescription(), m.Metadata.Description)
	assert.Equal(t, GetAuthor(), m.Metadata.Author)
	assert.Equal(t, GetURL(), m.Metadata.URL)
	assert.Equal(t, GetLicense(), m.Metadata.License)
	assert.Equal(t, GetCopyright(), m.Metadata.Copyright)
}

func TestGetMaintainers_ReturnsSlice(t *testing.T) {
	maintainers := GetMaintainers()
	// Slice may be empty in some environments; just confirm it doesn't panic and type is correct
	_ = maintainers
}

func TestGet_AfterReset_ReturnsFreshSingleton(t *testing.T) {
	first := Get()
	resetForTesting()
	second := Get()
	// After reset, a new singleton is created — but values should still be valid
	assert.NotNil(t, second)
	assert.NotEmpty(t, second.Version.Number)
	// Restore state
	resetForTesting()
	_ = first
}

func TestGet_InvalidYAML_FallsBackToDefault(t *testing.T) {
	// Save state and restore after the test.
	originalBytes := metadataByte
	defer func() {
		metadataByte = originalBytes
		resetForTesting()
		Get() // re-init singleton with valid data
	}()

	resetForTesting()
	metadataByte = []byte("key: [unclosed")
	m := Get()
	require.NotNil(t, m)
	assert.Equal(t, "Quiver", m.Metadata.Name, "should fall back to defaultMetadata")
}

func TestResolveHome_UserHomeDirFails_ReturnsRaw(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix test")
	}
	// Temporarily unset HOME so os.UserHomeDir fails, then restore.
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Unsetenv("HOME")

	// Override singleton to have a ~/ path so resolveHome tries UserHomeDir.
	original := Get()
	defer func() { metadata = original }()
	metadata = &Metadata{
		Version:  Version{Number: "0.0.0", Codename: "test"},
		Metadata: MetadataInfo{Name: "test"},
		Paths: Paths{
			Home:       OsValue[string]{Default: "~/.quiver"},
			Events:     "{{home}}/events",
			Store:      "{{home}}/store",
			Namespaces: "{{home}}/namespaces",
			Config:     "{{home}}/config.yaml",
			Vault:      "{{home}}/vault",
		},
	}
	result := GetHomePath()
	// When HOME is unset, UserHomeDir fails; resolveHome logs a warning and
	// returns the raw "~/.quiver" string (files land relative to cwd).
	assert.Equal(t, "~/.quiver", result)
}

func TestResolveHome_NonTildePath_ReturnsRaw(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix test")
	}
	// Save original and restore after test.
	original := Get()
	defer func() { metadata = original }()

	// Overwrite the singleton directly (once is already done, so Get() won't re-init).
	metadata = &Metadata{
		Version:  Version{Number: "0.0.0", Codename: "test"},
		Metadata: MetadataInfo{Name: "test"},
		Paths: Paths{
			Home:       OsValue[string]{Default: "/absolute/quiver"},
			Events:     "{{home}}/events",
			Store:      "{{home}}/store",
			Namespaces: "{{home}}/namespaces",
			Config:     "{{home}}/config.yaml",
			Vault:      "{{home}}/vault",
		},
	}
	result := GetHomePath()
	assert.Equal(t, "/absolute/quiver", result)
}

func TestGetEventsPathAt_UsesProvidedHome(t *testing.T) {
	home := t.TempDir()
	got := GetEventsPathAt(home)
	assert.Contains(t, got, home)
}

func TestGetStorePathAt_UsesProvidedHome(t *testing.T) {
	home := t.TempDir()
	got := GetStorePathAt(home)
	assert.Contains(t, got, home)
}

func TestGetNamespacesPathAt_UsesProvidedHome(t *testing.T) {
	home := t.TempDir()
	got := GetNamespacesPathAt(home)
	assert.Contains(t, got, home)
}

func TestGetVaultPathAt_UsesProvidedHome(t *testing.T) {
	home := t.TempDir()
	got := GetVaultPathAt(home)
	assert.Contains(t, got, home)
}
