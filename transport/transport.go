package transport

import (
	"context"
	"net"
)

type ServerTransport interface {
	HandleStreams(func(*Stream), func(context.Context, string) context.Context)
	Close() error
}

func NewServerTransport(protocol string, conn net.Conn, config *ServerConfig) (ServerTransport, error) {
	return newHTTP2Server(conn, config)
}



type Stream struct {
	method string
}

func (s *Stream) Method() string {
	return s.method
}


type ServerConfig struct {
	
}
