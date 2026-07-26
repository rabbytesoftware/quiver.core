package ws

import (
	"path"

	"github.com/gin-gonic/gin"
)

// defaultKeyParam is the route parameter every stream filtered on before a
// stream keyed by something other than a namespace existed.
const defaultKeyParam = "ns"

type StreamDef[T any] struct {
	// KeyParam names the route parameter carrying the stream key, and defaults
	// to "ns". A stream keyed by anything else sets it rather than naming its
	// route parameter :ns and pretending the key is a namespace.
	KeyParam  string
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

func BuildPredicate[T any](
	c *gin.Context,
	def StreamDef[T],
) func(T) bool {
	keyParam := def.KeyParam
	if keyParam == "" {
		keyParam = defaultKeyParam
	}
	keyPattern := c.Param(keyParam)

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
		if keyPattern != "" && !GlobMatch(keyPattern, def.Namespace(event)) {
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
