package commands_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func decodeCatalog(t *testing.T, out string) output.Catalog {
	t.Helper()

	var c output.Catalog
	require.NoError(t, json.Unmarshal([]byte(out), &c), "catalog payload: %s", out)

	return c
}

// list and search return the same payload; the query field is what
// distinguishes a filtered listing from a full one.
func TestCatalog_TotalCountsBothLists(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "json")
	require.NoError(t, err)

	c := decodeCatalog(t, out)

	assert.Len(t, c.Arrows, 1)
	assert.Len(t, c.Collections, 1)
	assert.Equal(t, len(c.Arrows)+len(c.Collections), c.Total)
}

func TestCatalog_UnfilteredListOmitsQuery(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "json")
	require.NoError(t, err)

	assert.Empty(t, decodeCatalog(t, out).Query)
	assert.NotContains(t, out, `"query"`)
}

func TestCatalog_SearchRecordsTheQuery(t *testing.T) {
	const pattern = "github.com/user/*"

	out, err := runCLI(t, &fakeDaemon{t: t}, "search", pattern, "-o", "json")
	require.NoError(t, err)

	assert.Equal(t, pattern, decodeCatalog(t, out).Query)
}

// A filter that matches nothing must still produce both keys as empty arrays,
// so a consumer iterating .arrows gets zero rows rather than a type error.
func TestCatalog_NoMatchesEncodesEmptyArrays(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "search", "gitlab.com/*", "-o", "json")
	require.NoError(t, err)

	c := decodeCatalog(t, out)

	assert.Equal(t, 0, c.Total)
	require.NotNil(t, c.Arrows)
	require.NotNil(t, c.Collections)
	assert.Contains(t, out, `"arrows": []`)
}

// The ref an arrow was registered under is the handle arrow remove expects, so
// the listing has to carry it.
func TestCatalog_ArrowRowCarriesRef(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "json")
	require.NoError(t, err)

	c := decodeCatalog(t, out)

	require.Len(t, c.Arrows, 1)
	assert.NotEmpty(t, c.Arrows[0].Ref)
	assert.NotEmpty(t, c.Arrows[0].State)
}

func TestCatalog_TableKeepsBothSections(t *testing.T) {
	out, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "table")
	require.NoError(t, err)

	assert.Contains(t, out, "ARROWS")
	assert.Contains(t, out, "COLLECTIONS")
	assert.Contains(t, out, "REF")
}

// An unfiltered listing does not print a result count: it would only restate
// the rows above it. A filtered one does, because that is the answer to "how
// much did the pattern keep".
func TestCatalog_TableShowsCountOnlyWhenFiltered(t *testing.T) {
	unfiltered, err := runCLI(t, &fakeDaemon{t: t}, "list", "-o", "table")
	require.NoError(t, err)
	assert.NotContains(t, unfiltered, "result(s)")

	filtered, err := runCLI(t, &fakeDaemon{t: t}, "search", "github.com/user/*", "-o", "table")
	require.NoError(t, err)
	assert.Contains(t, filtered, "result(s)")
}
