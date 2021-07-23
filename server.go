package RPC

import (
	"RPC/timer"
	"codec"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Option struct {
	CodecType codec.Type
}

type Server struct {
	serviceMap sync.Map
}

// 创建服务端
func NewServer(lis net.Listener) *Server {
	server := &Server{}
	go server.Accept(lis)
	return server
}

func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error: ", err)
			return
		}
		go server.ServerConn(conn)
	}
}

func (server *Server) ServerConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()

	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: option error:", err)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.serverCodec(f(conn))
}

// 接收并处理请求
type request struct {
	h            *codec.Header
	s            *service
	mtype        *methodType
	argv, replyv reflect.Value
}

func (method *methodType) newArgv() reflect.Value {
	var argv reflect.Value
	if method.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(method.ArgType.Elem())
	} else {
		argv = reflect.New(method.ArgType).Elem()
	}
	return argv
}

func (method *methodType) newReplyv() reflect.Value {
	replyv := reflect.New(method.ReplyType.Elem())
	switch method.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(method.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(method.ReplyType.Elem(), 0, 0))
	}
	return replyv

}

func (server *Server) serverCodec(c codec.Codec) {
	var wg sync.WaitGroup
	var sending sync.Mutex // 回复请求的报文必须逐个发送，并发容易导致多个回复报文交织在一起，使用锁保证。
	for {
		// 接收请求
		var header codec.Header
		if err := c.ReadHeader(&header); err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Println("rpc server: read header error: ", err)
			}
			break
		}

		// 查找服务和方法
		s, method, err := server.findService(header.ServiceMethod)
		if err != nil {
			header.Error = err.Error()
			server.sendResponse(c, &header, struct{}{}, &sending)
			continue
		}

		// 保存参数
		var req request
		req.h = &header
		req.s = s
		req.mtype = method
		req.argv = method.newArgv()
		req.replyv = method.newReplyv()

		argi := req.argv.Interface()
		if req.argv.Type().Kind() != reflect.Ptr {
			argi = req.argv.Addr().Interface()
		}
		if err := c.ReadBody(argi); err != nil {
			log.Println("rpc server: read argv error: " + err.Error())
			header.Error = "rpc server: read argv error: " + err.Error()
			server.sendResponse(c, &header, struct{}{}, &sending)
			continue
		}

		// 并发处理请求
		wg.Add(1)
		go server.handleRequest(c, &req, &sending, &wg)
	}
	wg.Wait()
}

// 统一使用serverCodec的互斥锁，来保证报文逐个发送
func (server *Server) sendResponse(c codec.Codec, header *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := c.Writer(header, body); err != nil {
		log.Println("rpc server: write response error: ", err)
	}
}

func (server *Server) handleRequest(c codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	f := req.mtype.method.Func

	called := make(chan struct{})
	go func() {
		returnValues := f.Call([]reflect.Value{req.s.rcvr, req.argv, req.replyv})
		called <- struct{}{}
		if errInter := returnValues[0].Interface(); errInter != nil {
			req.h.Error = errInter.(error).Error()
			server.sendResponse(c, req.h, struct{}{}, sending)
			return
		}
		server.sendResponse(c, req.h, req.replyv.Interface(), sending)
	}()

	select {
	case <-time.After(timer.ServiceHandleTimeOut):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: %s", timer.ServiceHandleTimeOut)
		server.sendResponse(c, req.h, struct{}{}, sending)
	case <-called:
		return
	}
}

// 服务注册
type methodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
}

type service struct {
	name   string
	typ    reflect.Type
	rcvr   reflect.Value // 结构体实例本身，调用时需要rcvr作为第0个参数
	method map[string]*methodType
}

func (server *Server) Register(rcvr interface{}) {
	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	s.name = reflect.Indirect(s.rcvr).Type().Name()
	if !ast.IsExported(s.name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.name)
	}

	s.method = make(map[string]*methodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)

		if !ast.IsExported(method.Name) ||
			method.Type.NumIn() != 3 || method.Type.NumOut() != 1 ||
			method.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() ||
			!isExportedOrBuiltinType(method.Type.In(1)) || !isExportedOrBuiltinType(method.Type.In(2)) {
			continue
		}

		s.method[method.Name] = &methodType{
			method:    method,
			ArgType:   method.Type.In(1),
			ReplyType: method.Type.In(2),
		}
		log.Printf("rpc server: register %s.%s", s.name, method.Name)
	}

	if _, ok := server.serviceMap.LoadOrStore(s.name, s); ok {
		log.Println("rpc server: service already defined ", s.name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

func (server *Server) findService(serviceMethod string) (s *service, method *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot == -1 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	si, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	s = si.(*service)

	method, ok = s.method[methodName]
	if !ok {
		err = errors.New("rpc server: can't find method" + serviceMethod)
	}
	return
}
