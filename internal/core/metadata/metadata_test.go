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
