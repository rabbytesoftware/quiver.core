package client_test

import (
	"net"
	"sync"
)

type countingConn struct {
	net.Conn
	once     sync.Once
	listener *countingListener
}

func (c *countingConn) Close() error {
	c.once.Do(c.listener.markClosed)
	return c.Conn.Close()
}
