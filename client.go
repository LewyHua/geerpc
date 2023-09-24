package geerpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"geerpc/coder"
	"log"
	"net"
	"sync"
)

// func (t *T) MethodName(argType T1, replyType *T2) error

// Call represents an active RPC
type Call struct {
	Seq           uint64      // 请求的序号
	ServiceMethod string      // 请求的服务名和方法名 例如 "Foo.Sum"
	Args          interface{} // 请求的参数
	Reply         interface{} // 请求的返回值
	Error         error       // 请求的错误
	Done          chan *Call  // 请求完成后会调用Done
}

// 把call自己传给done是为什么呢？ 为了让调用者知道哪个call已经完成了
// done方法会将call放入Done
func (c *Call) done() {
	c.Done <- c
}

// Client represents an RPC Client
type Client struct {
	cc       coder.Coder      // 用于发送请求
	opt      *Option          // 选项
	sending  sync.Mutex       // protect following
	header   coder.Header     // 请求的header
	mu       sync.Mutex       // protect following
	seq      uint64           // 请求的序号
	pending  map[uint64]*Call // 存储未处理完的call
	closing  bool             // 用户主动关闭
	shutdown bool             // 服务端关闭
}

var ErrShutdown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.cc.Close() // 关闭连接
}

// IsAvailable returns true if the client does work
func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closing && !c.shutdown
}

// registerCall registers a call
func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ErrShutdown
	}
	// 生成序号
	call.Seq = c.seq
	c.seq++
	// 存储call
	c.pending[call.Seq] = call

	return call.Seq, nil
}

// removeCall removes a call with given sequence number
func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	// 删除
	delete(c.pending, seq)
	// 返回call用来调用call.done()方法
	return call
}

// terminateCalls terminates all pending calls
func (c *Client) terminateCalls(err error) {
	// 为什么这里要保护header？
	// 因为这里是在发送请求的时候，如果发送请求的时候，header被修改了，那么就会出错
	// 这里是为了再删除calls的时候，不让客户端发送吗？
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	// 通知所有call
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var h coder.Header
		// 读取header
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		// 服务端已经处理完成，客户端接收到了header就可以删除call了
		call := c.removeCall(h.Seq)
		switch {
		case call == nil: // call 不存在，可能是请求没有发送完整，或者因为其他原因被取消，但是服务端仍旧处理了
			err = c.cc.ReadBody(nil)
		case h.Error != "": // call 存在，但服务端处理出错，即 h.Error 不为空
			call.Error = errors.New(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default: // call 存在，服务端处理正常，那么需要从 body 中读取 Reply 的值。
			err = c.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// 出错了，需要通知所有call
	c.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	coderFunc := coder.NewCoderFuncMap[opt.CoderType]
	// coder不存在
	if coderFunc == nil {
		err := fmt.Errorf("invalid coder type %s", opt.CoderType)
		log.Println("rpc client: coder error:", err)
		return nil, err
	}

	// send options
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error:", err)
		_ = conn.Close()
		return nil, err
	}
	// coderFunc 是一个NewCoderFunc的实例。调用它就返回一个Coder
	cc := coderFunc(conn)
	return newClientCoder(cc, opt), nil
}

func newClientCoder(cc coder.Coder, opt *Option) *Client {
	client := &Client{
		seq:     1,                      // seq 从 1 开始，0 表示无效的 Call
		cc:      cc,                     // 用于发送请求
		opt:     opt,                    // 选项
		pending: make(map[uint64]*Call), // 存储未处理完的call
	}
	// 开启一个goroutine来接收响应
	go client.receive()
	return client
}

// Go invokes the function asynchronously
func parseOptions(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CoderType == "" {
		opt.CoderType = DefaultOption.CoderType
	}
	return opt, nil
}

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	// 连接服务端
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// 如果client为nil，那么就关闭连接
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}

// send sends a request
func (c *Client) send(call *Call) {
	// 保护header不被修改
	c.sending.Lock()
	defer c.sending.Unlock()
	// 注册call
	seq, err := c.registerCall(call)
	// 注册失败，直接返回
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	// 设置header
	c.header.ServiceMethod = call.ServiceMethod
	c.header.Seq = seq
	c.header.Error = ""
	// 写入header 和 args
	if err := c.cc.Write(&c.header, call.Args); err != nil {
		// 写入失败，移除call
		call := c.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go invokes the function asynchronously
func (c *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	// 生成call
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	// 发送call
	c.send(call)
	return call
}

// Call invokes the named function, waits for it to complete, and returns its error status
func (c *Client) Call(serviceMethod string, args, reply interface{}) error {
	// 生成call
	call := <-c.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}

// 流程图
// client: Call -> send -> registerCall -> removeCall -> done
// server: readRequestHeader -> readRequest -> sendResponse -> readRequestHeader -> readRequest -> sendResponse ->
// client: receive -> removeCall -> done -> Call
