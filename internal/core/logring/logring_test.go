package logring_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func newLogger(ring *logring.Ring, level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var next bytes.Buffer
	return slog.New(ring.Wrap(slog.NewTextHandler(&next, &slog.HandlerOptions{Level: level}))), &next
}

func seqs(records []logring.Record) (out []uint64) {
	for _, r := range records {
		out = append(out, r.Seq)
	}
	return out
}

func TestRing_Stream_ReplaysTheLast500OrWhatFollowsSince(t *testing.T) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelInfo)
	for range 5100 {
		log.Info("m")
	}

	last, since, current, restarted := ring.Stream(0, slog.LevelInfo), ring.Stream(100, slog.LevelInfo), ring.Stream(5100, slog.LevelInfo), ring.Stream(9999, slog.LevelInfo)

	assert.Equal(t, uint64(4601), last.Replay[0].Seq)
	assert.Len(t, last.Replay, 500)
	assert.Equal(t, uint64(5100), last.Seq)
	assert.Equal(t, uint64(101), since.Replay[0].Seq, "the ring holds 5000: older records are gone")
	assert.Len(t, since.Replay, 5000)
	assert.Empty(t, current.Replay)
	assert.False(t, current.Reset)
	assert.True(t, restarted.Reset)
	assert.Len(t, restarted.Replay, 500, "a newer since means the daemon restarted: replay the last 500")
}

func TestRing_Stream_FiltersByLevelForReplayAndLive(t *testing.T) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelDebug)
	log.Debug("d")
	log.Warn("w")
	s := ring.Stream(0, slog.LevelWarn)
	defer s.Close()

	log.Info("i")
	log.Error("e")

	assert.Equal(t, []uint64{2}, seqs(s.Replay))
	assert.Equal(t, "warn", s.Replay[0].Level)
	assert.Equal(t, "e", (<-s.Live).Msg)
}

func TestRing_Stream_DropsASubscriberThatFallsBehindAndCloseIsIdempotent(t *testing.T) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelInfo)
	slow, closed := ring.Stream(0, slog.LevelInfo), ring.Stream(0, slog.LevelInfo)
	closed.Close()
	closed.Close()

	for range 300 {
		log.Info("m")
	}

	got := 0
	for range slow.Live {
		got++
	}
	_, open := <-closed.Live
	assert.Equal(t, 256, got)
	assert.False(t, open)
	slow.Close()
}

func TestRing_Wrap_RecordsStructuredFieldsAndPassesTheRecordOn(t *testing.T) {
	ring := logring.New()
	log, next := newLogger(ring, slog.LevelInfo)
	log = log.With("component", "release", "api_key", "k").WithGroup("g")

	log.Warn("hello", "n", 3, "ok", true, "err", assert.AnError, "long", strings.Repeat("x", 3000), "token", "t",
		slog.Group("in", "a", 1, "Password", "p"), slog.Group("", "inline", 1), "", "dropped")

	rec := ring.Stream(0, slog.LevelInfo).Replay[0]
	assert.Equal(t, "release", rec.Component)
	assert.Equal(t, "log", rec.Type)
	assert.Equal(t, map[string]any{
		"api_key": "[redacted]", "g.token": "[redacted]", "g.in.Password": "[redacted]",
		"g.in.a": int64(1), "g.inline": int64(1), "g.n": int64(3), "g.ok": true, "g.err": assert.AnError.Error(),
		"g.long": strings.Repeat("x", 2048),
	}, rec.Fields)
	assert.Contains(t, next.String(), "token=t", "redaction is for the ring only")
}

func TestRing_Wrap_CapsAttributesAndFollowsTheWrappedHandlerLevel(t *testing.T) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelWarn)
	args := make([]any, 0, 80)
	for i := range 40 {
		args = append(args, "k"+string(rune('A'+i)), i)
	}

	log.Info("quiet", args...)
	log.Warn("many", args...)

	got := ring.Stream(0, slog.LevelDebug).Replay
	require.Len(t, got, 1)
	assert.Len(t, got[0].Fields, 32)
	assert.False(t, log.Handler().Enabled(context.Background(), slog.LevelInfo))
}
