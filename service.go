package geerpc

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

type methodType struct {
	method    reflect.Method
	ArgvType  reflect.Type
	ReplyType reflect.Type
	numCalls  uint64
}

// NumCalls 返回调用次数
func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	// argv maybe a pointer type, or a value type
	if m.ArgvType.Kind() == reflect.Ptr {
		// 一个指向数据类型零值的value
		argv = reflect.New(m.ArgvType.Elem())
	} else {
		// 一个包含数据的value
		argv = reflect.New(m.ArgvType).Elem()
	}
	return argv
}

func (m *methodType) newReplyv() reflect.Value {
	// must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

type service struct {
	name     string
	typ      reflect.Type
	receiver reflect.Value
	method   map[string]*methodType
}

func NewService(receiver interface{}) *service {
	s := new(service)
	s.receiver = reflect.ValueOf(receiver)
	// 获取结构体的名字
	s.name = reflect.Indirect(s.receiver).Type().Name()
	s.typ = reflect.TypeOf(receiver)
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}
	s.registerMethods()
	return s
}

func (s *service) registerMethods() {
	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		// 一个方法必须有三个参数，第一个参数是receiver，第二个参数是argv，第三个参数是reply
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		// 返回值必须是error类型
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		// 第二个参数和第三个参数必须是导出或内置类型
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		// 新建一个methodType，存储方法的信息
		s.method[method.Name] = &methodType{
			method:    method,
			ArgvType:  argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.name, method.Name)
	}
}
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	// 调用方法
	retValues := f.Call([]reflect.Value{s.receiver, argv, replyv})
	// 返回值的第一个是error, 如果不为nil，就返回
	if errInter := retValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
