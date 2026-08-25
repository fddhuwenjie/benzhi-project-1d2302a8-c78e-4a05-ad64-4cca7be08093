package httpapi

import (
	"net"
	"net/http"
	"time"
)

func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
}

func Listen(server *http.Server) (net.Listener, error) { return net.Listen("tcp", server.Addr) }
