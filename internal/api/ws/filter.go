package ws

import (
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultKeyParam is the route parameter every stream filtered on before a
// stream keyed by something other than a namespace existed.
const defaultKeyParam = "ns"

type StreamDef[T any] struct {
	// KeyParam names the route parameter carrying the stream key, and defaults
	// to "ns". A stream keyed by anything else sets it rather than naming its
	// route parameter :ns and pretending the key is a namespace.
	KeyParam string
	// KeyMatch decides whether a subscriber's key pattern selects an event, and
	// defaults to GlobMatch. Globbing is meaningful for a namespace, where
	// `github.com/org/*` is a subscription a client deliberately asks for. It is
	// wrong for an opaque identifier: a job id has no hierarchy to match on, so
	// a glob there only lets one subscriber read every other job's results.
	// Streams keyed by an identifier set ExactMatch.
	KeyMatch  func(pattern, value string) bool
	Namespace func(T) string
	Serialize func(T) ([]byte, error)
	Filters   []FilterDef[T]
}

type FilterDef[T any] struct {
	Param   string
	Extract func(T) string
	Match   func(param, value string) bool
	Default string
}

func ExactMatch(param, value string) bool {
	return param == value
}

func GlobMatch(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	matched, err := path.Match(pattern, value)
	return err == nil && matched
}

// NamespaceMatch is GlobMatch plus the rule that a refless pattern selects
// every ref of that arrow.
//
// Events carry the namespace the arrow is catalogued under, which always has a
// ref, while a client subscribes with whatever the user typed — usually
// refless. Under a plain glob those never match, so a subscription made before
// firing a lifecycle method silently receives nothing and the caller waits for
// events that were all filtered out.
//
// The "@" is required rather than a bare prefix so that a subscription to
// github.com/user/app does not also collect github.com/user/app-extra.
func NamespaceMatch(pattern, value string) bool {
	if GlobMatch(pattern, value) {
		return true
	}

	if pattern == "" || strings.Contains(pattern, "@") {
		return false
	}

	ref, _, found := strings.Cut(value, "@")

	return found && GlobMatch(pattern, ref)
}

func BuildPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
) func(T) bool {
	keyParam := def.KeyParam
	if keyParam == "" {
		keyParam = defaultKeyParam
	}
	keyPattern := c.Param(keyParam)

	keyMatch := def.KeyMatch
	if keyMatch == nil {
		keyMatch = GlobMatch
	}

	type activeFilter struct {
		param string
		fd    FilterDef[T]
	}

	var active []activeFilter
	for _, f := range def.Filters {
		v := c.Query(f.Param)
		if v == "" {
			v = f.Default
		}
		if v != "" {
			active = append(active, activeFilter{param: v, fd: f})
		}
	}

	return func(event T) bool {
		if keyPattern != "" && !keyMatch(keyPattern, def.Namespace(event)) {
			return false
		}
		for _, af := range active {
			if !af.fd.Match(af.param, af.fd.Extract(event)) {
				return false
			}
		}
		return true
	}
}
