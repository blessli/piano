package transport

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/net/http2"
)

type http2Server struct {
	ctx                   context.Context
	conn                  net.Conn
	framer                *framer
	maxSendHeaderListSize *uint32
	controlBuf            *controlBuffer
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
	s := &Stream{
		method:         state.data.method,
	}
	handle(s)
	return false
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
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return nil, fmt.Errorf("transport: http2Server.HandleStreams saw invalid preface type %T from client", frame)
	}
	t.handleSettings(sf)
}

type controlBuffer struct {
	ch              chan struct{}
	done            <-chan struct{}
	mu              sync.Mutex
	consumerWaiting bool
	list            *itemList
	err             error

	// transportResponseFrames counts the number of queued items that represent
	// the response of an action initiated by the peer.  trfChan is created
	// when transportResponseFrames >= maxQueuedTransportResponseFrames and is
	// closed and nilled when transportResponseFrames drops below the
	// threshold.  Both fields are protected by mu.
	transportResponseFrames int
	trfChan                 atomic.Value // *chan struct{}
}

func (t *http2Server) handleSettings(f *http2.SettingsFrame) {
	if f.IsAck() {
		return
	}
	var ss []http2.Setting
	var updateFuncs []func()
	f.ForeachSetting(func(s http2.Setting) error {
		switch s.ID {
		case http2.SettingMaxHeaderListSize:
			updateFuncs = append(updateFuncs, func() {
				t.maxSendHeaderListSize = new(uint32)
				*t.maxSendHeaderListSize = s.Val
			})
		default:
			ss = append(ss, s)
		}
		return nil
	})
	t.controlBuf.executeAndPut(func(interface{}) bool {
		for _, f := range updateFuncs {
			f()
		}
		return true
	}, &incomingSettings{
		ss: ss,
	})
}
