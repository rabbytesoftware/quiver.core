package console

import (
	"context"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

const (
	logWriteTimeout = 5 * time.Second
	logPingInterval = 30 * time.Second
	logPongTimeout  = 60 * time.Second
	logReadLimit    = 1024
	watchInterval   = 250 * time.Millisecond
)

type logStream struct {
	conn       *websocket.Conn
	ring       logring.Ring
	params     logParams
	stuckLimit time.Duration
	last       uint64
}

func (s *logStream) run(
	ctx context.Context,
) {
	defer func() { _ = s.conn.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sub := s.ring.Subscribe(s.params.level)
	defer sub.Close()

	readDone := s.startReader()
	go s.watch(ctx, sub)

	if err := s.replay(); err != nil {
		return
	}

	ping := time.NewTicker(logPingInterval)
	defer ping.Stop()
	for s.step(ctx, sub, ping.C, readDone) {
	}
}

func (s *logStream) replay() error {
	since := s.params.since
	reset := since > s.ring.Latest()
	if reset {
		since = 0
	}

	s.last = since
	records := s.ring.Snapshot(since, s.params.level, s.params.replay)
	if err := s.sendAll(records); err != nil {
		return err
	}
	return s.write(readyFrame{Type: "ready", Seq: s.last, Reset: reset})
}

func (s *logStream) sendAll(
	records []logring.Record,
) error {
	if len(records) == 0 {
		return nil
	}
	if err := s.sendRecord(records[0]); err != nil {
		return err
	}
	return s.sendAll(records[1:])
}

func (s *logStream) step(
	ctx context.Context,
	sub logring.Subscription,
	pings <-chan time.Time,
	readDone <-chan struct{},
) bool {
	select {
	case rec, ok := <-sub.C():
		return ok && s.sendLive(sub, rec) == nil
	case <-pings:
		return s.ping() == nil
	case <-readDone:
		return false
	case <-ctx.Done():
		return false
	}
}

func (s *logStream) sendLive(
	sub logring.Subscription,
	rec logring.Record,
) error {
	if err := s.reportGap(sub); err != nil {
		return err
	}
	return s.sendRecord(rec)
}

func (s *logStream) reportGap(
	sub logring.Subscription,
) error {
	dropped := sub.TakeDropped()
	if dropped == 0 {
		return nil
	}
	return s.write(gapFrame{Type: "gap", Dropped: dropped})
}

func (s *logStream) sendRecord(
	rec logring.Record,
) error {
	if rec.Seq <= s.last {
		return nil
	}
	if err := s.write(logFrame{Type: "log", Record: rec}); err != nil {
		return err
	}
	s.last = rec.Seq
	return nil
}

func (s *logStream) write(
	frame any,
) error {
	_ = s.conn.SetWriteDeadline(time.Now().Add(logWriteTimeout))
	return s.conn.WriteJSON(frame)
}

func (s *logStream) ping() error {
	return s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(logWriteTimeout))
}

func (s *logStream) startReader() <-chan struct{} {
	done := make(chan struct{})
	s.conn.SetReadLimit(logReadLimit)
	_ = s.conn.SetReadDeadline(time.Now().Add(logPongTimeout))
	s.conn.SetPongHandler(func(string) error {
		return s.conn.SetReadDeadline(time.Now().Add(logPongTimeout))
	})
	go s.drain(done)
	return done
}

func (s *logStream) drain(
	done chan<- struct{},
) {
	defer close(done)
	for s.readOne() {
	}
}

func (s *logStream) readOne() bool {
	_, _, err := s.conn.NextReader()
	return err == nil
}

func (s *logStream) watch(
	ctx context.Context,
	sub logring.Subscription,
) {
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for s.watchOnce(ctx, sub, ticker.C) {
	}
}

func (s *logStream) watchOnce(
	ctx context.Context,
	sub logring.Subscription,
	ticks <-chan time.Time,
) bool {
	select {
	case <-ctx.Done():
		return false
	case <-ticks:
		return s.checkStuck(sub)
	}
}

func (s *logStream) checkStuck(
	sub logring.Subscription,
) bool {
	if sub.StuckFor() < s.stuckLimit {
		return true
	}

	message := websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "slow consumer")
	_ = s.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(logWriteTimeout))
	_ = s.conn.Close()
	return false
}
