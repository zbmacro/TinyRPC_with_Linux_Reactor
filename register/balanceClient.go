package register

// 支持负载均衡的客户端
// 负责实例化负载均衡类、获取客户端、调用client的Call/Go

import (
	"TinyRPC/client"
	"TinyRPC/server"
	"sync"
)

type BalanceClient struct {
	balance *Balance       // 负载均衡实例
	mode    SelectMode     // 负载均衡方式
	opt     *server.Option // 连接客户端的协商报文
	clients sync.Map       // map[服务器地址]*client.Client客户端实例
	mu      sync.Mutex
}

// Dial 创建包含负载均衡的客户端
func Dial(mode SelectMode, opts ...*server.Option) (*BalanceClient, error) {
	balance, err := NewBalance()
	if err != nil {
		return nil, err
	}
	c := &BalanceClient{
		balance: balance,
		mode:    mode,
		opt:     client.ParseOption(opts...),
	}
	return c, nil
}

// 通过负载均衡获取服务端地址，并创建连接
func (balanceC *BalanceClient) getClient(serviceName ServiceName) (*client.Client, error) {
	addr, err := balanceC.balance.Get(balanceC.mode, serviceName)
	if err != nil {
		return nil, err
	}

	cI, ok := balanceC.clients.Load(addr)
	if !ok || cI.(*client.Client).IsClose() {
		// 保证客户端不会重复创建
		balanceC.mu.Lock()
		defer balanceC.mu.Unlock()

		cI, ok = balanceC.clients.Load(addr)
		if !ok || cI.(*client.Client).IsClose() {
			c, err := client.Dial("tcp", string(addr), balanceC.opt)
			if err != nil {
				return nil, err
			}
			cI, ok = balanceC.clients.LoadOrStore(addr, c)
		}
	}

	return cI.(*client.Client), nil
}

// Call 同步请求
func (balanceC *BalanceClient) Call(serviceMethod string, argv, reply interface{}) error {
	c, err := balanceC.getClient(ServiceName(serviceMethod))
	if err != nil {
		return err
	}
	return c.Call(serviceMethod, argv, reply)
}

// Go 异步请求
func (balanceC *BalanceClient) Go(serviceMethod string, argv, reply interface{}) (*client.Call, error) {
	c, err := balanceC.getClient(ServiceName(serviceMethod))
	if err != nil {
		return nil, err
	}
	return c.Go(serviceMethod, argv, reply)
}
