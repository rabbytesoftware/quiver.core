package transports

import (
	"net"
	"os"
)

type socketListener struct {
	net.Listener
	path string
}

func (l *socketListener) Close() error {
	err := l.Listener.Close()
	_ = os.Remove(l.path)
	return err
}
