package output_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// keysOf marshals v with marshal, decodes the result, and returns its
// top-level keys. It mirrors the helper guarding the API DTOs, since the same
// tag-parity rule applies to every payload the CLI can emit.
func keysOf(t *testing.T, v any, marshal func(any) ([]byte, error)) []string {
	t.Helper()

	raw, err := marshal(v)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &decoded), "yaml decodes json too")

	keys := make([]string, 0, len(decoded))
	for k := range decoded {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// payloads is every shape this package can put on stdout. Both guarantees
// below run against all of them, so a new payload is covered by adding one
// line here.
func payloads() []struct {
	name  string
	value any
} {
	return []struct {
		name  string
		value any
	}{
		{"mutation", output.Mutation{
			Action:  output.ActionAdd,
			Subject: "github.com/u/r",
			At:      "2026-08-19T10:30:45Z",
		}},
		{"catalog", output.NewCatalog(
			[]output.ArrowRow{{Namespace: "github.com/u/r", Name: "r", Ref: "v1.0.0", State: "ready"}},
			[]output.CollectionRow{{Namespace: "github.com/u/c", Name: "c", Arrows: 3}},
			"pattern",
		)},
		{"arrow row", output.ArrowRow{Namespace: "github.com/u/r", Name: "r", Ref: "v1.0.0", State: "ready"}},
		{"collection row", output.CollectionRow{Namespace: "github.com/u/c", Name: "c", Arrows: 3}},
	}
}

// A payload carrying a json tag but no yaml tag serializes under two different
// key names, because yaml.v3 falls back to the lowercased Go field name. That
// would make -o yaml disagree with -o json for the same command.
func TestPayload_JSONAndYAMLKeysAgree(t *testing.T) {
	for _, tc := range payloads() {
		t.Run(tc.name, func(t *testing.T) {
			jsonKeys := keysOf(t, tc.value, func(v any) ([]byte, error) { return json.Marshal(v) })
			yamlKeys := keysOf(t, tc.value, func(v any) ([]byte, error) { return yaml.Marshal(v) })

			assert.Equal(t, jsonKeys, yamlKeys,
				"every payload field needs both a json and a yaml tag")
		})
	}
}

// Runner encodes a payload with yaml.v3, which panics rather than erroring on
// a type it cannot marshal. CheckPayload is where that contract is enforced.
func TestPayload_IsEncodable(t *testing.T) {
	for _, tc := range payloads() {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, tui.CheckPayload(tc.value))
		})
	}
}

func TestAction_Past(t *testing.T) {
	testCases := []struct {
		name   string
		action output.Action
		want   string
	}{
		{"add", output.ActionAdd, "added"},
		{"remove", output.ActionRemove, "removed"},
		{"refresh", output.ActionRefresh, "refreshed"},
		{"follow", output.ActionFollow, "followed"},
		{"unfollow", output.ActionUnfollow, "unfollowed"},
		{"update", output.ActionUpdate, "updated"},
		{"use", output.ActionUse, "switched to"},
		{"unknown falls back to the verb", output.Action("frobnicate"), "frobnicate"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.action.Past())
		})
	}
}

func TestNewCatalog_DerivesTotal(t *testing.T) {
	c := output.NewCatalog(
		[]output.ArrowRow{{Namespace: "a"}, {Namespace: "b"}},
		[]output.CollectionRow{{Namespace: "c"}},
		"",
	)

	assert.Equal(t, 3, c.Total)
}

// An empty catalog must encode as [] rather than null: a consumer iterating
// .arrows should get zero rows, not a type error.
func TestNewCatalog_NilListsEncodeAsEmptyArrays(t *testing.T) {
	c := output.NewCatalog(nil, nil, "")

	require.NotNil(t, c.Arrows)
	require.NotNil(t, c.Collections)
	assert.Equal(t, 0, c.Total)

	raw, err := json.Marshal(c)
	require.NoError(t, err)
	assert.JSONEq(t, `{"arrows":[],"collections":[],"total":0}`, string(raw))
}

// Query is omitempty: an unfiltered listing should not carry an empty filter.
func TestCatalog_UnfilteredOmitsQuery(t *testing.T) {
	raw, err := json.Marshal(output.NewCatalog(nil, nil, ""))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "query")
}
