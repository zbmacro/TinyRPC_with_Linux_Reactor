package xclient

// 支持服务注册和负载均衡
// 负责实例化负载均衡类、获取客户端、调用client的Call/Go

import (
	"RPC"
	"RPC/register"
	"RPC/timer"
	"errors"
	"net"
	"strings"
	"sync"
)

type XClient struct {
	clients map[string]*RPC.Client
	mu      sync.Mutex
	d       *register.Balance
	mode    register.SelectMode
	opt     *RPC.Option
}

// 创建包含负载均衡的客户端
func Dial(registerAddr string, mode register.SelectMode, opts ...*RPC.Option) (*XClient, error) {
	balance, err := register.NewBalance(registerAddr)
	if err != nil {
		return nil, err
	}
	client := &XClient{
		clients: make(map[string]*RPC.Client),
		d:       balance,
		opt:     RPC.ParseOption(opts...),
		mode:    mode,
	}
	return client, nil
}

// 同步请求
func (c *XClient) Call(serviceMethod string, argv, reply interface{}) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	return client.Call(serviceMethod, argv, reply)
}

// 异步请求
func (c *XClient) Go(serviceMethod string, argv, reply interface{}) (*RPC.Call, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	return client.Go(serviceMethod, argv, reply)
}

// 通过负载均衡获取服务端地址，并创建连接
func (c *XClient) getClient() (*RPC.Client, error) {
	addr, err := c.d.Get(c.mode)
	if err != nil {
		return nil, err
	}

	// 保证客户端不会重复创建
	c.mu.Lock()
	defer c.mu.Unlock()
	client, ok := c.clients[addr]
	if !ok || client.IsClose() {
		parts := strings.Split(addr, "@")
		if len(parts) != 2 {
			err = errors.New("is not a valid addr " + addr)
			return nil, err
		}
		var conn net.Conn
		// conn, err = net.Dial(parts[0], parts[1])
		conn, err = net.DialTimeout(parts[0], parts[1], timer.ConnectTimeOut)
		if err != nil {
			return nil, err
		}
		client, err = RPC.NewClient(conn, c.opt)
		if err != nil {
			return nil, err
		}
		c.clients[addr] = client
	}
	return client, err
}
