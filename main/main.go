package main

import (
	"fmt"
	"geerpc"
	"geerpc/client"
	"log"
	"net"
	"sync"
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
	// 开启了一个go协程调用receive 从client.cc.conn等待接收响应
	client, _ := client.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("geerpc req %d", i)
			var reply string
			// 往 从client.cc.conn 发送请求
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)
	}
	wg.Wait()
}
