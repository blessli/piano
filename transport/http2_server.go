package transport

import (
	"context"
	"fmt"
	"io"
	"net"
)

type http2Server struct {
	ctx    context.Context
	conn   net.Conn
	framer *framer
}

func newHTTP2Server(conn net.Conn, config *ServerConfig) (_ ServerTransport, err error) {
	framer := newFramer(conn)
	t := &http2Server{
		ctx:    context.Background(),
		conn:   conn,
		framer: framer,
	}
	t.framer.writer.Flush()
	frame, err := t.framer.fr.ReadFrame()
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("transport: http2Server.HandleStreams failed to read initial settings frame: %v", err)
	}
}
