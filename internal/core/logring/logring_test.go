package logring_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func logger(
	t *testing.T,
	ring logring.Ring,
) (*slog.Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	next := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(ring.Tee(next)), &buf
}

func TestRing_Snapshot_AssignsIncreasingSequenceNumbers(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.Info("one")
	log.Info("two")

	got := ring.Snapshot(0, slog.LevelDebug, 10)
	require.Len(t, got, 2)
	assert.Equal(t, uint64(1), got[0].Seq)
	assert.Equal(t, uint64(2), got[1].Seq)
	assert.Equal(t, uint64(2), ring.Latest())
}

func TestRing_Snapshot_WrapKeepsTheNewestRecords(t *testing.T) {
	ring := logring.New(3)
	log, _ := logger(t, ring)

	for _, msg := range []string{"a", "b", "c", "d", "e"} {
		log.Info(msg)
	}

	got := ring.Snapshot(0, slog.LevelDebug, 10)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"c", "d", "e"}, []string{got[0].Msg, got[1].Msg, got[2].Msg})
	assert.Equal(t, uint64(3), got[0].Seq)
}

func TestRing_Snapshot_SinceSkipsEarlierRecords(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	for _, msg := range []string{"a", "b", "c", "d"} {
		log.Info(msg)
	}

	got := ring.Snapshot(2, slog.LevelDebug, 10)

	require.Len(t, got, 2)
	assert.Equal(t, "c", got[0].Msg)
	assert.Equal(t, "d", got[1].Msg)
}

func TestRing_Snapshot_LimitKeepsTheMostRecent(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	for _, msg := range []string{"a", "b", "c", "d"} {
		log.Info(msg)
	}

	got := ring.Snapshot(0, slog.LevelDebug, 2)

	require.Len(t, got, 2)
	assert.Equal(t, "c", got[0].Msg)
	assert.Empty(t, ring.Snapshot(0, slog.LevelDebug, 0))
}

func TestRing_Snapshot_FiltersByLevel(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	log.Debug("d")
	log.Info("i")
	log.Warn("w")
	log.Error("e")

	got := ring.Snapshot(0, slog.LevelWarn, 10)

	require.Len(t, got, 2)
	assert.Equal(t, "warn", got[0].Level)
	assert.Equal(t, "error", got[1].Level)
}

func TestRing_New_ZeroCapacityStillHoldsOneRecord(t *testing.T) {
	ring := logring.New(0)
	log, _ := logger(t, ring)

	log.Info("a")
	log.Info("b")

	got := ring.Snapshot(0, slog.LevelDebug, 10)
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].Msg)
}

func TestRing_Tee_ForwardsEveryRecordToTheWrappedHandler(t *testing.T) {
	ring := logring.New(10)
	log, buf := logger(t, ring)

	log.Info("hello", "k", "v")

	assert.Contains(t, buf.String(), `"msg":"hello"`)
	assert.Contains(t, buf.String(), `"k":"v"`)
}

func TestRing_Tee_RespectsTheWrappedHandlersLevel(t *testing.T) {
	ring := logring.New(10)
	var buf bytes.Buffer
	next := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	log := slog.New(ring.Tee(next))

	log.Info("quiet")
	log.Warn("loud")

	got := ring.Snapshot(0, slog.LevelDebug, 10)
	require.Len(t, got, 1)
	assert.Equal(t, "loud", got[0].Msg)
}

func TestRing_Tee_CapturesTextHandlerOutputStructurally(t *testing.T) {
	ring := logring.New(10)
	var buf bytes.Buffer
	log := slog.New(ring.Tee(slog.NewTextHandler(&buf, nil)))

	log.Info("hi", "n", 3, "ok", true, "took", 1800*time.Millisecond, "ratio", 0.5)

	got := ring.Snapshot(0, slog.LevelDebug, 1)[0]
	assert.Equal(t, int64(3), got.Fields["n"])
	assert.Equal(t, true, got.Fields["ok"])
	assert.Equal(t, "1.8s", got.Fields["took"])
	assert.Equal(t, 0.5, got.Fields["ratio"])
}

func TestRing_Tee_ComponentLeavesTheFields(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.Warn("slow", "component", "release", "retry", 1)

	got := ring.Snapshot(0, slog.LevelDebug, 1)[0]
	assert.Equal(t, "release", got.Component)
	assert.NotContains(t, got.Fields, "component")
	assert.Equal(t, int64(1), got.Fields["retry"])
}

func TestRing_Tee_WithAttrsAndGroupsFlattenWithDottedKeys(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.With("component", "arrow").WithGroup("req").With("id", 7).Info("done", "path", "/x")
	log.Info("grouped", slog.Group("net", slog.String("addr", "a"), slog.Group("tls", slog.Bool("on", true))))

	got := ring.Snapshot(0, slog.LevelDebug, 2)
	assert.Equal(t, "arrow", got[0].Component)
	assert.Equal(t, int64(7), got[0].Fields["req.id"])
	assert.Equal(t, "/x", got[0].Fields["req.path"])
	assert.Equal(t, "a", got[1].Fields["net.addr"])
	assert.Equal(t, true, got[1].Fields["net.tls.on"])
}

func TestRing_Tee_ErrorAndStringerValuesBecomeStrings(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.Error("failed", "err", assert.AnError, "when", time.Date(2026, 10, 4, 14, 2, 0, 0, time.UTC))

	got := ring.Snapshot(0, slog.LevelDebug, 1)[0]
	assert.Equal(t, assert.AnError.Error(), got.Fields["err"])
	assert.Equal(t, "2026-10-04T14:02:00Z", got.Fields["when"])
}

func TestRing_Tee_CapsTheNumberOfFields(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	args := make([]any, 0, 80)
	for i := 0; i < 40; i++ {
		args = append(args, "k"+string(rune('A'+i%26))+string(rune('a'+i/26)), i)
	}

	log.Info("wide", args...)

	got := ring.Snapshot(0, slog.LevelDebug, 1)[0]
	assert.Len(t, got.Fields, 32)
	assert.True(t, got.FieldsTruncated)
}

func TestRing_Tee_TruncatesLongValuesOnARuneBoundary(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	long := strings.Repeat("é", 3000)

	log.Info("long", "v", long)

	value, ok := ring.Snapshot(0, slog.LevelDebug, 1)[0].Fields["v"].(string)
	require.True(t, ok)
	assert.LessOrEqual(t, len(value), 2048+len("…"))
	assert.True(t, strings.HasSuffix(value, "…"))
	assert.True(t, strings.HasPrefix(value, "é"))
}

func TestRing_Tee_RedactsSensitiveKeysInTheStoredRecord(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.Info("auth",
		"token", "abc", "Authorization", "Bearer xyz", "pairing_code", "123",
		slog.Group("db", slog.String("password", "p")), "note", "fine")

	fields := ring.Snapshot(0, slog.LevelDebug, 1)[0].Fields
	assert.Equal(t, logring.Redacted, fields["token"])
	assert.Equal(t, logring.Redacted, fields["Authorization"])
	assert.Equal(t, logring.Redacted, fields["pairing_code"])
	assert.Equal(t, logring.Redacted, fields["db.password"])
	assert.Equal(t, "fine", fields["note"])
}

func TestRing_Tee_RedactsCredentialsEmbeddedInValues(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)

	log.Info("fetch", "url", "https://user:hunter2@host/repo", "hdr", "sent Bearer abc.def-ghi ok")

	fields := ring.Snapshot(0, slog.LevelDebug, 1)[0].Fields
	assert.NotContains(t, fields["url"], "hunter2")
	assert.Contains(t, fields["url"], "https://"+logring.Redacted+"@host/repo")
	assert.NotContains(t, fields["hdr"], "abc.def-ghi")
	assert.Contains(t, fields["hdr"], "Bearer "+logring.Redacted)
}

func TestRing_Subscribe_ReceivesLiveRecordsAtOrAboveItsLevel(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	sub := ring.Subscribe(slog.LevelWarn)
	defer sub.Close()

	log.Info("skip")
	log.Warn("see")

	select {
	case rec := <-sub.C():
		assert.Equal(t, "see", rec.Msg)
	case <-time.After(time.Second):
		t.Fatal("no record delivered")
	}
}

func TestRing_Subscribe_FullQueueDropsAndCountsInsteadOfBlocking(t *testing.T) {
	ring := logring.New(1000)
	log, _ := logger(t, ring)
	sub := ring.Subscribe(slog.LevelDebug)
	defer sub.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 600; i++ {
			log.Info("x")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("producer blocked on a slow subscriber")
	}
	assert.Equal(t, uint64(600-256), sub.TakeDropped())
	assert.Zero(t, sub.TakeDropped())
	assert.Greater(t, sub.StuckFor(), time.Duration(0))
}

func TestRing_Subscribe_StuckForResetsOnceTheQueueAcceptsAgain(t *testing.T) {
	ring := logring.New(1000)
	log, _ := logger(t, ring)
	sub := ring.Subscribe(slog.LevelDebug)
	defer sub.Close()
	for i := 0; i < 300; i++ {
		log.Info("x")
	}
	require.Greater(t, sub.StuckFor(), time.Duration(0))

	for len(sub.C()) > 0 {
		<-sub.C()
	}
	log.Info("y")

	assert.Zero(t, sub.StuckFor())
}

func TestRing_Subscribe_CloseStopsDeliveryAndIsIdempotent(t *testing.T) {
	ring := logring.New(10)
	log, _ := logger(t, ring)
	sub := ring.Subscribe(slog.LevelDebug)

	sub.Close()
	sub.Close()
	log.Info("after")

	_, open := <-sub.C()
	assert.False(t, open)
}

func TestRing_Snapshot_ConcurrentWritersAndReadersAreRaceFree(t *testing.T) {
	ring := logring.New(64)
	log, _ := logger(t, ring)
	sub := ring.Subscribe(slog.LevelDebug)
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				log.Info("m", "j", j)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 200; j++ {
			ring.Snapshot(0, slog.LevelDebug, 10)
			ring.Latest()
		}
	}()
	wg.Wait()
	sub.Close()

	assert.Equal(t, uint64(800), ring.Latest())
}

func TestRing_Tee_HandleReturnsTheWrappedHandlersError(t *testing.T) {
	ring := logring.New(10)
	handler := ring.Tee(failingHandler{})

	err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0))

	require.Error(t, err)
	assert.Len(t, ring.Snapshot(0, slog.LevelDebug, 10), 1)
}

type failingHandler struct{ slog.Handler }

func (failingHandler) Enabled(context.Context, slog.Level) bool  { return true }
func (failingHandler) Handle(context.Context, slog.Record) error { return assert.AnError }
