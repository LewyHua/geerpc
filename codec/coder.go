package coder

import "io"

type Header struct {
	ServiceMethod string // format "Service.Method"
	Seq           uint64 // sequence number chosen by client
	Error         string
}

// Coder 编码器接口
type Coder interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type NewCoderFunc func(closer io.ReadWriteCloser) Coder

type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json"
)

var NewCoderFuncMap map[Type]NewCoderFunc

func init() {
	NewCoderFuncMap = make(map[Type]NewCoderFunc)
	NewCoderFuncMap[GobType] = NewGobCoder
}
