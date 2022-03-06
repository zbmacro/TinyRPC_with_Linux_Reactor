package server

import (
	"TinyRPC/reactor"
	"errors"
	"go/ast"
	"log"
	"reflect"
	"strings"
	"sync"
)

// Server 服务端实例
type Server struct {
	serviceMap sync.Map // 用于保存已注册的服务 map[服务名称]*reactor.Service
}

// New 创建服务端，需要调用Register注册RPC方法
func New() *Server {
	server := &Server{}
	return server
}

// 服务注册

// Register 注册结构体
func (server *Server) Register(rcvr interface{}) {
	s := new(reactor.Service)
	s.Typ = reflect.TypeOf(rcvr)
	s.Rcvr = reflect.ValueOf(rcvr)
	s.Name = reflect.Indirect(s.Rcvr).Type().Name()
	if !ast.IsExported(s.Name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.Name)
	}

	s.Method = make(map[string]*reactor.MethodType)
	for i := 0; i < s.Typ.NumMethod(); i++ {
		method := s.Typ.Method(i)

		// 查找结构体中符合规则的方法
		if !ast.IsExported(method.Name) ||
			method.Type.NumIn() != 3 || method.Type.NumOut() != 1 ||
			method.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() ||
			!isExportedOrBuiltinType(method.Type.In(1)) || !isExportedOrBuiltinType(method.Type.In(2)) {
			continue
		}

		s.Method[method.Name] = &reactor.MethodType{
			Method:    method,
			ArgType:   method.Type.In(1),
			ReplyType: method.Type.In(2),
		}
		log.Printf("rpc server: register %s.%s", s.Name, method.Name)
	}

	if _, ok := server.serviceMap.LoadOrStore(s.Name, s); ok {
		log.Println("rpc server: service already defined ", s.Name)
	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// 根据调用的服务名称，在服务实例中查找结构体及其方法信息
func (server *Server) findService(serviceMethod string) (s *reactor.Service, method *reactor.MethodType, err error) {
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
	s = si.(*reactor.Service)

	method, ok = s.Method[methodName]
	if !ok {
		err = errors.New("rpc server: can't find method" + serviceMethod)
	}
	return
}

// GetServices 获取服务列表
func (server *Server) GetServices() (services []string) {
	server.serviceMap.Range(func(key, sI interface{}) bool {
		s := sI.(*reactor.Service)

		s.Mu.Lock()
		for mkey := range s.Method {
			services = append(services, key.(string)+"."+mkey)
		}
		s.Mu.Unlock()

		return true
	})
	return
}
