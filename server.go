package geerpc

import (
	"encoding/json"
	"errors"
	"fmt"
	coder "geerpc/coder"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNUmber = 0x3bef5c

type Option struct {
	MagicNumber int
	CoderType   coder.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNUmber,
	CoderType:   coder.GobType,
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
	// 解码conn里面json的Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("RPC server: decode options err:", err)
		return
	}

	if opt.MagicNumber != MagicNUmber {
		log.Println("RPC server: invalid magic number:", opt.MagicNumber)
		return
	}

	// the NewGobCoder
	coderFunc, ok := coder.NewCoderFuncMap[opt.CoderType]
	if !ok {
		log.Println("RPC server: invalid code type:", opt.CoderType)
		return
	}
	s.serveCoder(coderFunc(conn))
}

// invalidRequest the placeholder in response when err occurred
var invalidRequest = struct{}{}

func (s *Server) serveCoder(cc coder.Coder) {
	sending := new(sync.Mutex) // make sure to send a complete response
	wg := new(sync.WaitGroup)
	for {
		// 读取数据到request
		req, err := s.readRequest(cc)
		if err != nil {
			// header读取失败，直接返回
			if req == nil {
				break
			}
			// body读取失败，发送错误
			req.header.Error = err.Error()
			// 返回响应必须是逐个发送的，用mutex来约束
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
	header *coder.Header // header of request
	argv   reflect.Value // argv of request
	replyv reflect.Value // replyv of request
}

func (s *Server) readRequestHeader(cc coder.Coder) (*coder.Header, error) {
	var header coder.Header
	// 从conn里面读取数据，存到header里面
	if err := cc.ReadHeader(&header); err != nil {
		if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			log.Println("RPC server: read header err:", err)
		}
		return nil, err
	}
	return &header, nil
}

// Read Request Format:
// | Option | Header1 | Body1 | Header2 | Body2 | ...
func (s *Server) readRequest(cc coder.Coder) (*request, error) {
	header, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{header: header}
	// TODO: we do not know the type of the request argv
	// just assume it as a string
	req.argv = reflect.New(reflect.TypeOf(""))
	// 从conn读取body，设置到req.argv.Interface()里面
	if err = cc.ReadBody(req.argv.Interface()); err != nil {
		log.Println("RPC server: read argv err:", err)
	}
	return req, nil
}

func (s *Server) sendResponse(cc coder.Coder, header *coder.Header, body interface{}, sending *sync.Mutex) {
	// 同时有响应err和正确的方法call，所以需要轮流发，一面消息粘在一起
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(header, body); err != nil {
		log.Println("RPC server: write response err:", err)
	}
}

func (s *Server) handleRequest(cc coder.Coder, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO: should call registered rpc methods to get the right replyv
	// just print argv and send hello for now
	defer wg.Done()
	log.Println(req.header, "+", req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.header.Seq))
	s.sendResponse(cc, req.header, req.replyv.Interface(), sending)
}
