package geerpc

import (
	"fmt"
	"reflect"
	"testing"
)

type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

func TestNewService(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	_assert(len(s.method) == 1, "wrong service Method, expect 1, but got %d", len(s.method))
	mType := s.method["Sum"]
	_assert(mType != nil, "failed to register method Sum")
}

func TestMethodType_Call(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	mType := s.method["Sum"]

	argv := mType.newArgv()
	replyv := mType.newReplyv()
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.call(mType, argv, replyv)
	// 检查是否调用成功 以及 返回值是否正确 以及 调用次数是否正确
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.NumCalls() == 1, "failed to call Foo.Sum")
}

//func startServer(addr chan string) {
//	//var foo Foo
//	//if err := Register(&foo); err != nil {
//	//	fmt.Println("register failed")
//	//}
//	//// pick a free port
//	//listener, err := net.Listen("tcp", ":0")
//	//if err != nil {
//	//	fmt.Println("network error:", err)
//	//	return
//	//}
//	//// notify
//	//log.Println("start rpc server on", listener.Addr())
//	//addr <- listener.Addr().String()
//	//Accept(listener)
//	var b Bar
//	_ = Register(&b)
//	// pick a free port
//	l, _ := net.Listen("tcp", ":0")
//	addr <- l.Addr().String()
//	Accept(l)
//}
