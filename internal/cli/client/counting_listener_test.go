package client_test

import (
	"net"
	"sync/atomic"
)

type countingListener struct {
	net.Listener
	accepted atomic.Int32
	closed   atomic.Int32
	signal   chan struct{}
}

func newCountingListener(
	inner net.Listener,
) *countingListener {
	return &countingListener{Listener: inner, signal: make(chan struct{}, 1024)}
}

func (l *countingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.accepted.Add(1)
	return &countingConn{Conn: conn, listener: l}, nil
}

func (l *countingListener) markClosed() {
	l.closed.Add(1)
	l.signal <- struct{}{}
}

func (l *countingListener) open() int {
	return int(l.accepted.Load() - l.closed.Load())
}
