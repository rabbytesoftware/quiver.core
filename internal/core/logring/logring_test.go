package logring_test

import (
	"bytes"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func newLogger(
	ring logring.Ring,
	level slog.Level,
) (*slog.Logger, *bytes.Buffer) {
	var next bytes.Buffer
	return slog.New(ring.Wrap(slog.NewTextHandler(&next, &slog.HandlerOptions{Level: level}))), &next
}

func logAlternating(
	log *slog.Logger,
	count int,
) {
	for i := 1; i <= count; i++ {
		logOne(log, i)
	}
}

func logOne(
	log *slog.Logger,
	seq int,
) {
	if seq%2 == 0 {
		log.Warn("even")
		return
	}
	log.Info("odd")
}

func assertReplay(
	t *testing.T,
	total int,
	since uint64,
	level slog.Level,
	wantFirst uint64,
	wantLen int,
	wantReset bool,
) {
	t.Helper()

	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelDebug)
	logAlternating(log, total)
	name := strconv.Itoa(total) + " since " + strconv.FormatUint(since, 10) + " " + level.String()

	stream := ring.Stream(since, level)
	defer stream.Close()

	require.Len(t, stream.Replay(), wantLen, name)
	assert.Equal(t, wantReset, stream.Reset(), name)
	assert.Equal(t, uint64(total), stream.Seq(), name)
	if wantLen > 0 {
		assert.Equal(t, wantFirst, stream.Replay()[0].Seq, name)
	}
}

func TestRing_Stream_SinceZero_ReplaysTheLast500(
	t *testing.T,
) {
	assertReplay(t, 5100, 0, slog.LevelInfo, 4601, 500, false)
	assertReplay(t, 300, 0, slog.LevelInfo, 1, 300, false)
	assertReplay(t, 0, 0, slog.LevelInfo, 0, 0, false)
}

func TestRing_Stream_SinceSet_ReplaysWhatTheRingStillHolds(
	t *testing.T,
) {
	assertReplay(t, 10, 3, slog.LevelInfo, 4, 7, false)
	assertReplay(t, 10, 9, slog.LevelInfo, 10, 1, false)
	assertReplay(t, 5100, 100, slog.LevelInfo, 101, 5000, false)
	assertReplay(t, 5100, 4000, slog.LevelInfo, 4001, 1100, false)
}

func TestRing_Stream_SinceCurrent_ReplaysNothing(
	t *testing.T,
) {
	assertReplay(t, 10, 10, slog.LevelInfo, 0, 0, false)
}

func TestRing_Stream_SinceNewerThanTheNewestRecord_ResetsAndReplaysTheLast500(
	t *testing.T,
) {
	assertReplay(t, 5100, 9999, slog.LevelInfo, 4601, 500, true)
	assertReplay(t, 10, 11, slog.LevelInfo, 1, 10, true)
}

func TestRing_Stream_Level_FiltersTheReplay(
	t *testing.T,
) {
	assertReplay(t, 10, 0, slog.LevelWarn, 2, 5, false)
	assertReplay(t, 10, 5, slog.LevelWarn, 6, 3, false)
	assertReplay(t, 10, 0, slog.LevelError, 0, 0, false)
	assertReplay(t, 5100, 0, slog.LevelWarn, 4602, 250, false)
}

func TestRing_Stream_Live_FiltersByLevel(
	t *testing.T,
) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelDebug)
	stream := ring.Stream(0, slog.LevelWarn)
	defer stream.Close()

	log.Info("i")
	log.Error("e")

	assert.Equal(t, "e", (<-stream.Live()).Msg)
}

func TestRing_Stream_SlowReader_IsDroppedAndTheLiveChannelCloses(
	t *testing.T,
) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelInfo)
	slow := ring.Stream(0, slog.LevelInfo)
	logAlternating(log, 300)

	delivered := 0
	for range slow.Live() {
		delivered++
	}

	assert.Equal(t, 256, delivered)
	slow.Close()
}

func TestRing_Stream_Close_IsIdempotentAndEndsTheLiveChannel(
	t *testing.T,
) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelInfo)
	stream := ring.Stream(0, slog.LevelInfo)

	stream.Close()
	stream.Close()
	log.Info("after close")

	_, open := <-stream.Live()
	assert.False(t, open)
}

func TestRing_Wrap_Record_KeepsStructuredFieldsAndRedactsSensitiveKeys(
	t *testing.T,
) {
	ring := logring.New()
	log, next := newLogger(ring, slog.LevelInfo)
	log = log.With("component", "release", "api_key", "k").WithGroup("g")

	log.Warn("hello", "n", 3, "ok", true, "err", assert.AnError, "long", strings.Repeat("x", 3000), "token", "t",
		slog.Group("in", "a", 1, "Password", "p"), slog.Group("", "inline", 1), "", "dropped")

	stream := ring.Stream(0, slog.LevelInfo)
	defer stream.Close()
	record := stream.Replay()[0]
	assert.Equal(t, "release", record.Component)
	assert.Equal(t, "log", record.Type)
	assert.Equal(t, "warn", record.Level)
	assert.Equal(t, map[string]any{
		"api_key": "[redacted]", "g.token": "[redacted]", "g.in.Password": "[redacted]",
		"g.in.a": int64(1), "g.inline": int64(1), "g.n": int64(3), "g.ok": true, "g.err": assert.AnError.Error(),
		"g.long": strings.Repeat("x", 2048),
	}, record.Fields)
	assert.Contains(t, next.String(), "token=t", "redaction is for the ring only")
}

func TestRing_Wrap_Record_KeepsAtMost32Fields(
	t *testing.T,
) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelInfo)
	args := make([]any, 0, 80)
	for i := 0; i < 40; i++ {
		args = append(args, "k"+strconv.Itoa(i), i)
	}

	log.Info("many", args...)

	stream := ring.Stream(0, slog.LevelInfo)
	defer stream.Close()
	assert.Len(t, stream.Replay()[0].Fields, 32)
}

func TestRing_Wrap_Enabled_FollowsTheWrappedHandler(
	t *testing.T,
) {
	ring := logring.New()
	log, _ := newLogger(ring, slog.LevelWarn)

	log.Info("quiet")

	stream := ring.Stream(0, slog.LevelDebug)
	defer stream.Close()
	assert.False(t, log.Handler().Enabled(context.Background(), slog.LevelInfo))
	assert.Empty(t, stream.Replay())
}

func TestRing_Wrap_Handle_StoresTheRecordThenReturnsTheWrappedHandlersError(
	t *testing.T,
) {
	ring := logring.New()
	handler := ring.Wrap(failingHandler{})
	record := slog.NewRecord(time.Now(), slog.LevelError, "lost", 0)

	err := handler.Handle(context.Background(), record)

	stream := ring.Stream(0, slog.LevelDebug)
	defer stream.Close()
	assert.ErrorIs(t, err, errWrapped)
	require.Len(t, stream.Replay(), 1)
	assert.Equal(t, "lost", stream.Replay()[0].Msg)
}
