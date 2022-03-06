package TinyRPC

import (
	"TinyRPC/client"
	"TinyRPC/config"
	"TinyRPC/reactor"
	"TinyRPC/register"
	"TinyRPC/server"
	"log"
)

// NewServer 创建服务端
func NewServer(addr string) *server.Server {
	s := server.New()
	go func() {
		if err := reactor.Reactor(addr, s); err != nil {
			log.Fatalln(err)
		}
	}()
	return s
}

// ServerStartRegisterClient 服务端启动注册中心客户端实例，注册服务及发送心态
// 请调用server.Register后再调用此方法，将会读取所注册的所用服务信息并发送
// addr 服务器地址
func ServerStartRegisterClient(addr string, s *server.Server) error {
	c, err := NewClient(config.RegisterAddr)
	if err != nil {
		return err
	}
	heartbeat := &register.Heartbeat{
		Addr:   addr,
		Client: c,
	}
	services := s.GetServices()
	var servicesName []register.ServiceName

	for _, name := range services {
		servicesName = append(servicesName, register.ServiceName(name))
	}

	postInfo := register.PostInfo{
		Address:      register.Addr(addr),
		ServicesName: servicesName,
	}
	if err := heartbeat.SendServices(postInfo); err != nil {
		return err
	}

	heartbeat.SendHeartbeat()
	return nil
}

// NewClient 创建客户端
func NewClient(addr string, opts ...*server.Option) (*client.Client, error) {
	return client.Dial("tcp", addr, opts...)
}

// NewClientByBalance 基于负载均衡的方式创建客户端
func NewClientByBalance(mode register.SelectMode, opts ...*server.Option) (*register.BalanceClient, error) {
	return register.Dial(mode, opts...)
}

// NewRegister 创建注册中心
func NewRegister() {
	s := NewServer(config.RegisterAddr)
	register.NewRegister(s)
}
