package geerpc

import (
	"encoding/json"
	"errors"
	"geerpc/coder"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNUmber = 0x3bef5c

type Option struct {
	MagicNumber    int
	CoderType      coder.Type
	ConnectTimeout time.Duration // 0 means no limit
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNUmber,
	CoderType:      coder.GobType,
	ConnectTimeout: time.Second * 10,
}

// Server RPC Server
type Server struct {
	serviceMap sync.Map // map[string]*service
}

// NewServer returns a new RPC Server
func NewServer() *Server {
	return &Server{}
}

// DefaultServer default RPC server instance
var DefaultServer = NewServer()

// Register 服务器注册服务 receiver是一个结构体指针
func (s *Server) Register(receiver interface{}) error {
	svc := NewService(receiver)
	if _, exist := s.serviceMap.LoadOrStore(svc.name, svc); exist {
		return errors.New("rpc: service already defined: " + svc.name)
	}
	return nil
}

// Register 对外暴露的注册服务的方法
func Register(receiver interface{}) error {
	return DefaultServer.Register(receiver)
}

func (s *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	// 从serviceMethod里面解析出service和method
	dot := strings.LastIndex(serviceMethod, ".")
	// 没有找到.，返回错误
	if dot < 0 {
		err = errors.New("rpc: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	serviceInstance, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc: can't find service " + serviceName)
		return
	}
	// 转换成service类型
	svc = serviceInstance.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc: can't find method " + methodName)
	}
	return
}

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
	s.serveCoder(coderFunc(conn), &opt)
}

// invalidRequest the placeholder in response when err occurred
var invalidRequest = struct{}{}

func (s *Server) serveCoder(cc coder.Coder, opt *Option) {
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
		go s.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
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
	mType  *methodType   // methodType of request
	svc    *service
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
	req.svc, req.mType, err = s.findService(header.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mType.newArgv()
	req.replyv = req.mType.newReplyv()

	// make sure argv is a pointer type, or it will panic
	argvi := req.argv.Interface()
	if req.argv.Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("RPC server: read argv err:", err)
		return req, err
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

func (s *Server) handleRequest(cc coder.Coder, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mType, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.header.Error = err.Error()
			s.sendResponse(cc, req.header, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(cc, req.header, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()

	// 如果没有设置超时，那么就直接返回
	if timeout == 0 {
		<-called
		<-sent
		return
	}

	select {
	// timeout结束，call还没有接收到数据，直接sendResponse
	case <-time.After(timeout):
		req.header.Error = "rpc server: handle request timeout"
		s.sendResponse(cc, req.header, invalidRequest, sending)
	// call接收到数据，说明已经发送了，直接返回
	case <-called:
		<-sent
	}
}
