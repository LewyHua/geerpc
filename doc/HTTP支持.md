## HTTP支持
> 开启HTTP支持之后，用户可以通过HTTP网页端看到RPC调用的统计信息
> 也可以通过HTTP客户端调用RPC服务

### 服务端支持
- 服务器开启一个handler负责监听defaultRPCPath，检查是否是CONNECT请求
1. 如果是则返回200，并调用ServeConn开始处理请求
2. 否则返回405 Method Not Allowed
```go
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    if req.Method != "CONNECT" {
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusMethodNotAllowed)
        _, _ = w.Write([]byte("405 must CONNECT\n"))
        return
    }
    // w.(http.Hijacker).Hijack() 从http连接中获取底层的net.Conn
    conn, _, err := w.(http.Hijacker).Hijack()
    if err != nil {
        log.Print("rpc hijacking ", req.RemoteAddr, ": ", err.Error())
        return
    }
    _, _ = io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
    s.ServeConn(conn)
}

// HandleHTTP registers an HTTP handler for RPC messages on rpcPath.
// It is still necessary to invoke http.Serve(), typically in a go statement.
func (s *Server) HandleHTTP() {
    http.Handle(defaultRPCPath, s)
    http.Handle(defaultDebugPath, debugHTTP{s})
    log.Println("rpc server debug path:", defaultDebugPath)
}
```

### 客户端支持
- 启动一个HTTP客户端，通过CONNECT请求与服务端建立连接
1. 如果服务端返回200，则开始处理请求
2. 否则返回错误
```go
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
    _, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))
    
    // Require successful HTTP response
    // before switching to RPC protocol.
    resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
    if err == nil && resp.Status == connected {
        return NewClient(conn, opt)
    }
    if err == nil {
        err = errors.New("unexpected HTTP response: " + resp.Status)
    }
    return nil, err
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string, opts ...*Option) (*Client, error) {
    return dialTimeout(NewHTTPClient, network, address, opts...)
}
```

- 客户端对Dial的封装
1. 通过@符号分割协议和地址
2. 根据协议调用不同的Dial/DialHTTP函数
   1. Dial/DialHTTP函数调用dialTimeout函数，并传入对应类型的NewClient/NewHTTPClient函数
   2. dialTimeout函数通过传入的network和addr调用net.DialTimeout函数，返回一个net.Conn
   3. 通过这个conn创建一个Client
```go
func XDial(rpcAddr string, opts ...*Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client: invalid format %s", rpcAddr)
	}
	// tcp@localhost:1234
	network, addr := parts[0], parts[1]
	switch network {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		return Dial(network, addr, opts...)
	}
}

```