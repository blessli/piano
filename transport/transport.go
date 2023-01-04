package transport

import (
	"context"
	"io"
	"net"
)

type ServerTransport interface {
	HandleStreams(func(*Stream), func(context.Context, string) context.Context)
	Close() error
	Write(s *Stream, hdr []byte, data []byte) error
}

func NewServerTransport(protocol string, conn net.Conn, config *ServerConfig) (ServerTransport, error) {
	return newHTTP2Server(conn, config)
}

type Stream struct {
	id           uint32
	method         string
	contentSubtype string
	trReader       io.Reader
}

func (s *Stream) Method() string {
	return s.method
}

func (s *Stream) ContentSubtype() string {
	return s.contentSubtype
}

func (s *Stream) Read(p []byte) (n int, err error) {
	return io.ReadFull(s.trReader, p)
}

type ServerConfig struct {
}
