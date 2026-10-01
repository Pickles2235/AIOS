package webui

import (
	"net"
	"os"
	"sync"
)

// Both sockets bind specific loopback addresses. A wildcard listener would
// expose HTTP to the LAN even if the application still checked origins.
type dualLoopback struct {
	v4, v6      net.Listener
	connections chan net.Conn
	done        chan struct{}
	once        sync.Once
}

func listenDualLoopback() (net.Listener, error) {
	v4, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(v4.Addr().String())
	v6, err := net.Listen("tcp6", net.JoinHostPort("::1", port))
	if err != nil {
		v4.Close()
		return nil, err
	}
	l := &dualLoopback{v4: v4, v6: v6, connections: make(chan net.Conn), done: make(chan struct{})}
	for _, socket := range []net.Listener{v4, v6} {
		go func(socket net.Listener) {
			for {
				conn, err := socket.Accept()
				if err != nil {
					l.Close()
					return
				}
				select {
				case l.connections <- conn:
				case <-l.done:
					conn.Close()
					return
				}
			}
		}(socket)
	}
	return l, nil
}

func (l *dualLoopback) Addr() net.Addr { return l.v4.Addr() }
func (l *dualLoopback) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.done:
		return nil, os.ErrClosed
	}
}
func (l *dualLoopback) Close() error {
	l.once.Do(func() { close(l.done); l.v4.Close(); l.v6.Close() })
	return nil
}
