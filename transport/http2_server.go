package transport

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"

	"golang.org/x/net/http2"
)

type http2Server struct {
	ctx                   context.Context
	conn                  net.Conn
	framer                *framer
	maxSendHeaderListSize *uint32
}

func (t *http2Server) HandleStreams(handle func(*Stream), traceCtx func(context.Context, string) context.Context) {
	for {
		frame, err := t.framer.fr.ReadFrame()
		if err != nil {
			return
		}
		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
			if t.operateHeaders(frame, handle, traceCtx) {
				break
			}
		default:
			log.Printf("transport: http2Server.HandleStreams found unhandled frame type %v.", frame)
		}
	}
}
func (t *http2Server) operateHeaders(frame *http2.MetaHeadersFrame, handle func(*Stream), traceCtx func(context.Context, string) context.Context) (fatal bool) {
	state := &decodeState{
		serverSide: true,
	}
	if err := state.decodeHeader(frame); err != nil {
		return false
	}
	s := &Stream{
		method: state.data.method,
	}
	handle(s)
	return false
}
func newHTTP2Server(conn net.Conn, config *ServerConfig) (_ ServerTransport, err error) {
	framer := newFramer(conn)
	isettings := []http2.Setting{{
		ID:  http2.SettingMaxFrameSize,
		Val: http2MaxFrameLen,
	}}
	if err := framer.fr.WriteSettings(isettings...); err != nil {
		return nil, fmt.Errorf("newHTTP2Server|WriteSettings: %v", err)
	}
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
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return nil, fmt.Errorf("transport: http2Server.HandleStreams saw invalid preface type %T from client", frame)
	}
}

func (t *http2Server) Write(s *Stream, hdr []byte, data []byte) error {
	// Add some data to header frame so that we can equally distribute bytes across frames.
	emptyLen := http2MaxFrameLen - len(hdr)
	if emptyLen > len(data) {
		emptyLen = len(data)
	}
	hdr = append(hdr, data[:emptyLen]...)
	data = data[emptyLen:]
	dataItem := &dataFrame{
		streamID: s.id,
		h:        hdr,
		d:        data,
	}
	var buf []byte
	if len(dataItem.h) != 0 { // data header has not been written out yet.
		buf = dataItem.h
	} else {
		buf = dataItem.d
	}
	size := http2MaxFrameLen
	if len(buf) < size {
		size = len(buf)
	}
	if err := t.framer.fr.WriteData(dataItem.streamID, true, buf[:size]); err != nil {
		return err
	}
	return nil
}
