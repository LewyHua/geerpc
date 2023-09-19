package main

import (
	"encoding/json"
	"fmt"
	"geerpc"
	coder "geerpc/codec"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {
	// pick a free port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("network err:", err)
		return
	}
	log.Println("start rpc server on ", listener.Addr())
	addr <- listener.Addr().String()
	geerpc.Accept(listener)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	// client
	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	_ = json.NewEncoder(conn).Encode(geerpc.DefaultOption)
	cc := coder.NewGobCoder(conn)
	// send and receive
	for i := 0; i < 5; i++ {
		h := &coder.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("geerpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
