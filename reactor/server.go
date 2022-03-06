package reactor

import (
	"TinyRPC/codec"
	"TinyRPC/network"
	"reflect"
	"sync"
)

// HandlerReadTask subReactor发送任务给handlerRead池，同时也用于server.SelectCodec和server.ServerCodec方法反序列化数据
type HandlerReadTask struct {
	Fd           int
	Conn         *network.Conn // 读取协商报文
	C            codec.Codec   // 序列化报文
	CMu          sync.Mutex    // 确保codec不会重复确认导致错误
	Sending      sync.Mutex    // 确保同一连接send操作串行
	SubReactorer *SubReactor   // subReactor实例，用于操作epoll监听的事件
}

// WorkerTask handlerRead池发送任务给worker池，同时用于server.HandleRequest方法处理业务逻辑
// WorkerTask worker池发送任务给handlerWriteControl，同时用于server.SendResponse方法处理响应
type WorkerTask struct {
	Fd           int
	C            codec.Codec
	Req          *Request
	Sending      *sync.Mutex // 确保同一连接send操作串行
	SubReactorer *SubReactor // subReactor实例，用于操作epoll监听的事件
}

// Request 反序列化数据结果，返回给handlerRead，用于worker处理业务逻辑
type Request struct {
	H            *codec.Header
	S            *Service
	Mtype        *MethodType
	Argv, Replyv reflect.Value
}

// Service 注册的结构体信息
type Service struct {
	Name   string
	Typ    reflect.Type
	Rcvr   reflect.Value // 结构体实例本身，调用时需要rcvr作为第0个参数
	Method map[string]*MethodType
	Mu     sync.Mutex
}

// MethodType 注册的结构体中的方法
type MethodType struct {
	Method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
}

type Server interface {
	SelectCodec(t *HandlerReadTask) error
	ServerCodec(t *HandlerReadTask) (req *Request, err error)
	HandleRequest(handle *WorkerTask, handlerWriteTask chan *WorkerTask)
	SendResponse(handle *WorkerTask) (err error)
}
