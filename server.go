package piano

import (
	"context"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"

	"github.com/blessli/piano/encoding/proto"
	"github.com/blessli/piano/transport"
	"golang.org/x/net/trace"
	"google.golang.org/grpc/encoding"
)

type Server struct {
	opts  *Options
	mu    sync.Mutex
	lis   map[net.Listener]bool
	conns map[transport.ServerTransport]bool
	m     map[string]*service
}
type service struct {
	server interface{} // the server for service methods
	md     map[string]*MethodDesc
	mdata  interface{}
}

type methodHandler func(srv any, ctx context.Context, dec func(any) error) (any, error)

type MethodDesc struct {
	MethodName string
	Handler    methodHandler
}
type ServiceDesc struct {
	ServiceName string
	HandlerType any
	Methods     []MethodDesc
	Metadata    any
}

func (s *Server) RegisterService(sd *ServiceDesc, ss interface{}) {
	ht := reflect.TypeOf(sd.HandlerType).Elem()
	st := reflect.TypeOf(ss)
	if !st.Implements(ht) {
		log.Fatalf("grpc: Server.RegisterService found the handler of type %v that does not satisfy %v", st, ht)
	}
	s.register(sd, ss)
}

func (s *Server) register(sd *ServiceDesc, ss interface{}) {
	if _, ok := s.m[sd.ServiceName]; ok {
		log.Fatalf("grpc: Server.RegisterService found duplicate service registration for %q", sd.ServiceName)
	}
	srv := &service{
		server: ss,
		md:     make(map[string]*MethodDesc),
		mdata:  sd.Metadata,
	}
	for i := range sd.Methods {
		d := &sd.Methods[i]
		srv.md[d.MethodName] = d
	}
	s.m[sd.ServiceName] = srv
}

func (s *Server) Serve(lis net.Listener) error {
	for {
		rawConn, err := lis.Accept()
		if err != nil {
			return err
		}
		go func() {
			s.handleRawConn(rawConn)
		}()
	}
}

func (s *Server) handleRawConn(rawConn net.Conn) {
	st := s.newHTTP2Transport(rawConn)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[st] = true
	go func() {
		s.serveStreams(st)
		s.removeConn(st)
	}()

}
func (s *Server) newHTTP2Transport(c net.Conn) transport.ServerTransport {
	config := &transport.ServerConfig{}
	st, err := transport.NewServerTransport("http2", c, config)
	if err != nil {
		s.mu.Lock()
		log.Printf("NewServerTransport(%q) failed: %v\n", c.RemoteAddr(), err)
		s.mu.Unlock()
		c.Close()
		log.Printf("grpc: Server.Serve failed to create ServerTransport: ", err)
		return nil
	}

	return st
}
func (s *Server) removeConn(st transport.ServerTransport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns != nil {
		delete(s.conns, st)
	}
}
func (s *Server) serveStreams(st transport.ServerTransport) {
	defer st.Close()
	var wg sync.WaitGroup
	st.HandleStreams(func(stream *transport.Stream) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleStream(st, stream)
		}()
	}, func(ctx context.Context, method string) context.Context {
		tr := trace.New("grpc.Recv."+method, method)
		return trace.NewContext(ctx, tr)
	})
	wg.Wait()
}

func (s *Server) handleStream(t transport.ServerTransport, stream *transport.Stream) {
	sm := stream.Method()
	pos := strings.LastIndex(sm, "/")
	service := sm[:pos]
	method := sm[pos+1:]
	srv, knownService := s.m[service]
	if knownService {
		if md, ok := srv.md[method]; ok {
			s.processUnaryRPC(t, stream, srv, md)
			return
		}
	}
}
func (s *Server) getCodec(contentSubtype string) baseCodec {
	if s.opts.codec != nil {
		return s.opts.codec
	}
	if contentSubtype == "" {
		return encoding.GetCodec(proto.Name)
	}
	codec := encoding.GetCodec(contentSubtype)
	if codec == nil {
		return encoding.GetCodec(proto.Name)
	}
	return codec
}
func (s *Server) processUnaryRPC(t transport.ServerTransport, stream *transport.Stream, srv *service, md *MethodDesc) (err error) {
	p := &parser{r: stream}
	_, d, err := p.recvMsg()
	if err != nil {
		return err
	}
	df := func(v interface{}) error {
		if err := s.getCodec(stream.ContentSubtype()).Unmarshal(d, v); err != nil {
			return fmt.Errorf("grpc: error unmarshalling request: %v", err)
		}

		return nil
	}
	reply, appErr := md.Handler(srv.server, context.TODO(), df)
	if appErr != nil {
		return appErr
	}
	if err := s.sendResponse(t, stream, reply); err != nil {
		return err
	}
	return nil
}

func (s *Server) sendResponse(t transport.ServerTransport, stream *transport.Stream, msg interface{}) error {
	data, err := encode(s.getCodec(stream.ContentSubtype()), msg)
	if err != nil {
		return err
	}
	compData := data
	hdr, payload := msgHeader(data, compData)
	return t.Write(stream, hdr, payload)
}
func NewServer(opt ...Option) *Server {
	opts := &Options{}
	for _, o := range opt {
		o(opts)
	}
	s := &Server{
		lis:   make(map[net.Listener]bool),
		opts:  opts,
		conns: make(map[transport.ServerTransport]bool),
		m:     make(map[string]*service),
	}
	return s
}
