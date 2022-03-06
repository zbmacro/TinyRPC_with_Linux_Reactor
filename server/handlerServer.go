package server

import (
	"TinyRPC/codec"
	"TinyRPC/reactor"
	"errors"
	"golang.org/x/sys/unix"
	"log"
	"reflect"
)

// Option 协商报文
type Option struct {
	CodecType codec.Type // 客户端所使用的序列化方式
}

// SelectCodec 用于handler池处理用户选择序列化方式
func (server *Server) SelectCodec(t *reactor.HandlerReadTask) error {
	c := t.Conn

	// 使用json方式发送所选择的序列化方式
	json := codec.NewCodecFuncMap["json"](c)
	var opt Option
	if err := json.ReadBody(&opt); err != nil {
		return err
	}

	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		return errors.New("close")
	}

	t.C = f(c) // 反序列化实例，一个连接的共享一个，因为部分序列化程序需要上下文信息，比如gob
	return nil
}

// ServerCodec 用于handlerRead池反序列化数据
func (server *Server) ServerCodec(t *reactor.HandlerReadTask) (req *reactor.Request, err error) {
	c := t.C
	// 接收请求
	var header codec.Header
	req = &reactor.Request{}
	req.H = &header

	if err = c.ReadHeader(&header); err != nil {
		if err != unix.EAGAIN && err.Error() != "close" {
			// 致命错误
			header.Error = "rpc server: read header error: " + err.Error()
		}
		return
	}

	// 查找服务和方法
	s, method, err := server.findService(header.ServiceMethod)
	if err != nil {
		header.Error = err.Error()
		return req, nil // 非致命错误
	}

	// 保存参数
	req.S = s
	req.Mtype = method
	req.Argv = newArgv(method)
	req.Replyv = newReplyv(method)

	argi := req.Argv.Interface()
	if req.Argv.Type().Kind() != reflect.Ptr {
		argi = req.Argv.Addr().Interface()
	}
	if err = c.ReadBody(argi); err != nil {
		if err != unix.EAGAIN && err.Error() != "close" {
			// 致命错误
			header.Error = "rpc server: read argv error: " + err.Error()
		}
		return
	}
	return
}

func newArgv(method *reactor.MethodType) reflect.Value {
	var argv reflect.Value
	if method.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(method.ArgType.Elem())
	} else {
		argv = reflect.New(method.ArgType).Elem()
	}
	return argv
}

func newReplyv(method *reactor.MethodType) reflect.Value {
	replyv := reflect.New(method.ReplyType.Elem())
	switch method.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(method.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(method.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

// HandleRequest 用于worker池处理业务逻辑
func (server *Server) HandleRequest(handle *reactor.WorkerTask, handlerWriteTask chan *reactor.WorkerTask) {
	if handle.Req.H.Error != "" {
		handlerWriteTask <- handle
		return
	}
	f := handle.Req.Mtype.Method.Func

	returnValues := f.Call([]reflect.Value{handle.Req.S.Rcvr, handle.Req.Argv, handle.Req.Replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		handle.Req.H.Error = errInter.(error).Error()
		handlerWriteTask <- handle
		return
	}
	handlerWriteTask <- handle
}

// SendResponse 用于handlerWriter池发送响应
func (server *Server) SendResponse(handle *reactor.WorkerTask) (err error) {
	var reply interface{}
	if handle.Req.H.Error == "" {
		reply = handle.Req.Replyv.Interface()
	}
	if err = handle.C.Writer(handle.Req.H, reply); err != nil {
		log.Println("rpc server: write response error: ", err)
	}
	return err
}
