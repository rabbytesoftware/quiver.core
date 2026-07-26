package ws_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	ws "github.com/rabbytesoftware/quiver.core/internal/api/ws"
)

func TestExactMatch(t *testing.T) {
	assert.True(t, ws.ExactMatch("true", "true"))
	assert.False(t, ws.ExactMatch("true", "false"))
	assert.False(t, ws.ExactMatch("", "true"))
}

func TestGlobMatch_Exact(t *testing.T) {
	assert.True(t, ws.GlobMatch("github.com/user/repo", "github.com/user/repo"))
	assert.False(t, ws.GlobMatch("github.com/user/repo", "github.com/user/other"))
}

func TestGlobMatch_Wildcard(t *testing.T) {
	assert.True(t, ws.GlobMatch("github.com/user/*", "github.com/user/repo"))
	assert.True(t, ws.GlobMatch("github.com/user/*", "github.com/user/other"))
	assert.False(t, ws.GlobMatch("github.com/other/*", "github.com/user/repo"))
}

func TestGlobMatch_Empty_MatchesAll(t *testing.T) {
	assert.True(t, ws.GlobMatch("", "github.com/user/repo"))
	assert.True(t, ws.GlobMatch("", "anything"))
}

func TestGlobMatch_InvalidPattern_NoMatch(t *testing.T) {
	assert.False(t, ws.GlobMatch("[invalid", "github.com/user/repo"))
}

type testEvent struct {
	ns    string
	color string
}

var testDef = ws.StreamDef[testEvent]{
	Namespace: func(e testEvent) string { return e.ns },
	Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
	Filters: []ws.FilterDef[testEvent]{
		{
			Param:   "color",
			Extract: func(e testEvent) string { return e.color },
			Match:   ws.ExactMatch,
		},
	},
}

func ginCtx(nsParam string, query url.Values) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?"+query.Encode(), nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "ns", Value: nsParam}}
	return c
}

func TestBuildPredicate_NoFilters_MatchesAll(t *testing.T) {
	pred := ws.BuildPredicate(ginCtx("", url.Values{}), testDef)
	assert.True(t, pred(testEvent{ns: "anything", color: "red"}))
}

func TestBuildPredicate_NamespaceGlob(t *testing.T) {
	pred := ws.BuildPredicate(ginCtx("github.com/user/*", url.Values{}), testDef)
	assert.True(t, pred(testEvent{ns: "github.com/user/repo"}))
	assert.False(t, pred(testEvent{ns: "github.com/other/repo"}))
}

func TestBuildPredicate_FieldFilter(t *testing.T) {
	pred := ws.BuildPredicate(ginCtx("", url.Values{"color": {"red"}}), testDef)
	assert.True(t, pred(testEvent{ns: "any", color: "red"}))
	assert.False(t, pred(testEvent{ns: "any", color: "blue"}))
}

func TestBuildPredicate_NamespaceAndFieldFilter(t *testing.T) {
	pred := ws.BuildPredicate(ginCtx("github.com/user/*", url.Values{"color": {"red"}}), testDef)
	assert.True(t, pred(testEvent{ns: "github.com/user/repo", color: "red"}))
	assert.False(t, pred(testEvent{ns: "github.com/user/repo", color: "blue"}))
	assert.False(t, pred(testEvent{ns: "github.com/other/repo", color: "red"}))
}

func TestBuildPredicate_AbsentQueryParam_Skipped(t *testing.T) {
	pred := ws.BuildPredicate(ginCtx("", url.Values{}), testDef)
	assert.True(t, pred(testEvent{ns: "any", color: "red"}))
	assert.True(t, pred(testEvent{ns: "any", color: "blue"}))
}

func TestBuildPredicate_DefaultFieldFilter(t *testing.T) {
	def := ws.StreamDef[testEvent]{
		Namespace: func(e testEvent) string { return e.ns },
		Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
		Filters: []ws.FilterDef[testEvent]{
			{
				Param:   "color",
				Extract: func(e testEvent) string { return e.color },
				Match:   ws.ExactMatch,
				Default: "red",
			},
		},
	}
	pred := ws.BuildPredicate(ginCtx("", url.Values{}), def)
	assert.True(t, pred(testEvent{ns: "any", color: "red"}))
	assert.False(t, pred(testEvent{ns: "any", color: "blue"}))
}

func keyedCtx(key, value string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	if key != "" {
		c.Params = gin.Params{{Key: key, Value: value}}
	}
	return c
}

// TestBuildPredicate_EmptyKeyParamStillFiltersOnNS is the regression guard for
// the arrow, runtime and collection streams, which never set KeyParam.
func TestBuildPredicate_EmptyKeyParamStillFiltersOnNS(t *testing.T) {
	pred := ws.BuildPredicate(keyedCtx("ns", "github.com/user/repo"), testDef)
	assert.True(t, pred(testEvent{ns: "github.com/user/repo"}))
	assert.False(t, pred(testEvent{ns: "github.com/other/repo"}))
}

func TestBuildPredicate_CustomKeyParamFiltersOnThatParam(t *testing.T) {
	def := ws.StreamDef[testEvent]{
		KeyParam:  "job",
		Namespace: func(e testEvent) string { return e.ns },
		Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
	}

	pred := ws.BuildPredicate(keyedCtx("job", "job-a"), def)
	assert.True(t, pred(testEvent{ns: "job-a"}))
	assert.False(t, pred(testEvent{ns: "job-b"}))
}

// TestBuildPredicate_CustomKeyParamIgnoresNS proves the key really moved: a
// route that still carries :ns must not filter on it once KeyParam names
// something else.
func TestBuildPredicate_CustomKeyParamIgnoresNS(t *testing.T) {
	def := ws.StreamDef[testEvent]{
		KeyParam:  "job",
		Namespace: func(e testEvent) string { return e.ns },
		Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
	}

	pred := ws.BuildPredicate(keyedCtx("ns", "job-a"), def)
	assert.True(t, pred(testEvent{ns: "job-b"}))
}

func TestBuildPredicate_UnsetParamMatchesEverything(t *testing.T) {
	def := ws.StreamDef[testEvent]{
		KeyParam:  "job",
		Namespace: func(e testEvent) string { return e.ns },
		Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
	}

	pred := ws.BuildPredicate(keyedCtx("", ""), def)
	assert.True(t, pred(testEvent{ns: "job-a"}))
	assert.True(t, pred(testEvent{ns: "job-b"}))
}

func TestBuildPredicate_DefaultOverriddenByQueryParam(t *testing.T) {
	def := ws.StreamDef[testEvent]{
		Namespace: func(e testEvent) string { return e.ns },
		Serialize: func(e testEvent) ([]byte, error) { return nil, nil },
		Filters: []ws.FilterDef[testEvent]{
			{
				Param:   "color",
				Extract: func(e testEvent) string { return e.color },
				Match:   ws.ExactMatch,
				Default: "red",
			},
		},
	}
	pred := ws.BuildPredicate(ginCtx("", url.Values{"color": {"blue"}}), def)
	assert.False(t, pred(testEvent{ns: "any", color: "red"}))
	assert.True(t, pred(testEvent{ns: "any", color: "blue"}))
}
