package ws

import (
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultKeyParam is the route parameter every stream filtered on before a
// stream keyed by something other than a namespace existed.
const defaultKeyParam = "ns"

// OverflowPolicy is what a broadcaster does when a subscriber's send buffer is
// full.
type OverflowPolicy int

const (
	// OverflowDrop discards the frame and logs it. It suits streams whose
	// subscribers reconcile through REST, where the next event supersedes the
	// lost one.
	OverflowDrop OverflowPolicy = iota
	// OverflowDisconnect closes the subscriber with 1013 (try again later).
	// It suits streams that carry each item exactly once and can replay: a
	// reconnect recovers everything that was missed, whereas a silent drop
	// would leave the client with a hole it cannot detect.
	OverflowDisconnect
)

type StreamDef[T any] struct {
	// Overflow selects what happens to a subscriber that cannot keep up. The
	// zero value is OverflowDrop.
	Overflow OverflowPolicy
	// Seq numbers a stream's events, strictly increasing per key. It is only
	// consulted alongside Replay, to stitch the replay to the live feed
	// without a duplicate or a gap.
	Seq func(T) uint64
	// Replay returns what a key has already emitted, oldest first, and is
	// called once per connection, after the subscriber is registered for live
	// events. A stream without it delivers only what is pushed after connect.
	Replay func(key string) []T
	// Done reports that a key will emit nothing more. When it closes, the
	// broadcaster flushes the subscriber and closes the socket with a normal
	// closure (1000, "completed"): the close frame is the stream's terminal
	// signal, so the stream keeps carrying exactly one payload type.
	Done func(key string) <-chan struct{}
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
