package codec

import "io"

type Header struct {
	ServiceMethod string // 请求方法
	Seq           uint64 // 请求编号，客户端异步请求时的标识
	Error         string // 服务端错误通过header传回
}

// Codec 序列化器 要实现的接口
type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Writer(*Header, interface{}) error
}

type Type string
type NewCodecFunc func(io.ReadWriteCloser) Codec

// NewCodecFuncMap 序列化器 通过写入NewCodecFuncMap实现注册
var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
}

func Register(t Type, f NewCodecFunc) {
	NewCodecFuncMap[t] = f
}
