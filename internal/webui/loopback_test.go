package webui

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestDualLoopbackSamePortHTTPAndClose(t *testing.T) {
	l, err := listenDualLoopback()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("owned-loopback")) })}
	t.Cleanup(func() { server.Close() })
	go server.Serve(l)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, host := range []string{"127.0.0.1", "::1"} {
		response, err := client.Get("http://" + net.JoinHostPort(host, port))
		if err != nil {
			t.Fatal("real address family unreachable", host, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || string(body) != "owned-loopback" {
			t.Fatal("address families do not share HTTP authority")
		}
	}
	dual := l.(*dualLoopback)
	for _, socket := range []net.Listener{dual.v4, dual.v6} {
		host, _, _ := net.SplitHostPort(socket.Addr().String())
		if !net.ParseIP(host).IsLoopback() {
			t.Fatal("wildcard/LAN socket opened")
		}
	}
	l.Close()
	if _, err = l.Accept(); err == nil {
		t.Fatal("closed listener accepted new connections")
	}
}
