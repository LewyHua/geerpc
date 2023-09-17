package geerpc

import (
	"encoding/json"
	"fmt"
	"geerpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNUmber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodeType    codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNUmber,
	CodeType:    codec.GobType,
}

// Server RPC Server
type Server struct{}

// NewServer returns a new RPC Server
func NewServer() *Server {
	return &Server{}
}

// DefaultServer default RPC server instance
var DefaultServer = NewServer()

// Accept accepts connections of the listener and serve it
func (s *Server) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("RPC server: accept err:", err)
			return
		}
		go s.ServeConn(conn)
	}
}

// ServeConn serve the connection
func (s *Server) ServeConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("RPC server: decode options err:", err)
		return
	}

	if opt.MagicNumber != MagicNUmber {
		log.Println("RPC server: invalid magic number:", opt.MagicNumber)
		return
	}

	f, ok := codec.NewCodeFuncMap[opt.CodeType]
	if !ok {
		log.Println("RPC server: invalid code type:", opt.CodeType)
		return
	}

	s.serveCodec(f(conn))
}

// invalidRequest the placeholder in response when err occurred
var invalidRequest = struct{}{}

func (s *Server) serveCodec(cc codec.Codec) {
	sending := new(sync.Mutex) // make sure to send a complete response
	wg := new(sync.WaitGroup)
	for {
		req, err := s.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.header.Error = err.Error()
			s.sendResponse(cc, req.header, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go s.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()

}

// Accept accepts connections of the listener and serve it
func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}

type request struct {
	header *codec.Header // header of request
	argv   reflect.Value // argv of request
	replyv reflect.Value // replyv of request
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var header codec.Header
	if err := cc.ReadHeader(&header); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("RPC server: read header err:", err)
		}
		return nil, err
	}
	return &header, nil
}

func (s *Server) readRequest(cc codec.Codec) (*request, error) {
	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{header: header}
	// TODO: we do not know the type of the request argv
	// just assume it as a string
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("RPC server: read argv err:", err)
	}
	return req, nil

}

func (s *Server) sendResponse(cc codec.Codec, header *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(header, body); err != nil {
		log.Println("RPC server: write response err:", err)
	}
}

func (s *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO: should call registered rpc methods to get the right replyv
	// just print argv and send hello for now
	defer wg.Done()
	log.Println(req.header, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.header.Seq))
	s.sendResponse(cc, req.header, req.replyv.Interface(), sending)
}
