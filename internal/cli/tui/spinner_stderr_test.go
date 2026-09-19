package tui_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// syncBuffer is a concurrency-safe writer for asserting spinner output.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestSpinner_FastPathWritesNothing(t *testing.T) {
	buf := &syncBuffer{}
	sp := tui.NewSpinner(buf, "loading", 50*time.Millisecond)
	sp.Start()
	sp.Stop() // stops before the 50ms delay elapses
	assert.Empty(t, buf.String(), "an op that finishes before the delay must render nothing")
}

func TestSpinner_DrawsLabelThenClears(t *testing.T) {
	buf := &syncBuffer{}
	sp := tui.NewSpinner(buf, "loading", 1*time.Millisecond)
	sp.Start()
	assert.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "loading")
	}, time.Second, 5*time.Millisecond, "spinner should draw the label after the delay")
	sp.Stop()
	assert.True(t, strings.HasSuffix(buf.String(), "\r\033[K"),
		"Stop must clear the line after drawing")
}

func TestSpinner_StopIsIdempotent(t *testing.T) {
	buf := &syncBuffer{}
	sp := tui.NewSpinner(buf, "x", 1*time.Millisecond)
	sp.Start()
	sp.Stop()
	sp.Stop() // must not panic
}
