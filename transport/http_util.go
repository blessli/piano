package transport

import (
	"bufio"
	"net"
	"strconv"

	"go.starlark.net/lib/proto"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	http2MaxFrameLen         = 16384
	writeBufferSize          = 32 * 1024
	readBufferSize           = 32 * 1024
	http2InitHeaderTableSize = 4096
	maxHeaderListSize        = 16777216
)

type bufWriter struct {
	buf       []byte
	offset    int
	batchSize int
	conn      net.Conn
	err       error

	onFlush func()
}

func newBufWriter(conn net.Conn, batchSize int) *bufWriter {
	return &bufWriter{
		buf:       make([]byte, batchSize*2),
		batchSize: batchSize,
		conn:      conn,
	}
}

func (w *bufWriter) Write(b []byte) (n int, err error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.batchSize == 0 { // Buffer has been disabled.
		return w.conn.Write(b)
	}
	for len(b) > 0 {
		nn := copy(w.buf[w.offset:], b)
		b = b[nn:]
		w.offset += nn
		n += nn
		if w.offset >= w.batchSize {
			err = w.Flush()
		}
	}
	return n, err
}

func (w *bufWriter) Flush() error {
	if w.err != nil {
		return w.err
	}
	if w.offset == 0 {
		return nil
	}
	if w.onFlush != nil {
		w.onFlush()
	}
	_, w.err = w.conn.Write(w.buf[:w.offset])
	w.offset = 0
	return w.err
}

type framer struct {
	writer *bufWriter
	fr     *http2.Framer
}

func newFramer(conn net.Conn) *framer {
	r := bufio.NewReaderSize(conn, readBufferSize)
	w := newBufWriter(conn, writeBufferSize)
	f := &framer{
		writer: w,
		fr:     http2.NewFramer(w, r),
	}
	f.fr.SetMaxReadFrameSize(http2MaxFrameLen)
	f.fr.SetReuseFrames()
	f.fr.MaxHeaderListSize = maxHeaderListSize
	f.fr.ReadMetaHeaders = hpack.NewDecoder(http2InitHeaderTableSize, nil)
	return f
}

type decodeState struct {
	// whether decoding on server side or not
	serverSide bool

	// Records the states during HPACK decoding. It will be filled with info parsed from HTTP HEADERS
	// frame once decodeHeader function has been invoked and returned.
	data parsedHeaderData
}

type parsedHeaderData struct {
	encoding       string
	method         string
	isGRPC         bool
	contentSubtype string
}

func (d *decodeState) decodeHeader(frame *http2.MetaHeadersFrame) error {
	for _, hf := range frame.Fields {
		d.processHeaderField(hf)
	}
	return nil
}

func (d *decodeState) processHeaderField(f hpack.HeaderField) {
	switch f.Name {
	case "content-type":
		d.data.contentSubtype = ""
		d.data.isGRPC = true
	case "grpc-encoding":
		d.data.encoding = f.Value
	case ":path":
		d.data.method = f.Value
	default:
	}
}
