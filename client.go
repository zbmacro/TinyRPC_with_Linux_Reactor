package RPC

// 不支持服务注册和负载均衡，xclient支持

import (
	"RPC/timer"
	"codec"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

type Call struct {
	serviceMethod string
	seq           uint64
	argv, reply   interface{}
	Error         error
	done          chan *Call
}

type Client struct {
	c       codec.Codec
	seq     uint64
	pending map[uint64]*Call
	mu      sync.Mutex // 确保seq的并发安全
	sending sync.Mutex // 请求的报文必须逐个发送
	closing bool
}

var DefaultOption = &Option{
	CodecType: "gob",
}

// 关闭连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closing {
		c.closing = true
		c.c.Close()
	}
}

func (c *Client) IsClose() bool {
	return c.closing
}

// 创建客户端
func NewClient(conn net.Conn, opt *Option) (client *Client, err error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err = fmt.Errorf("invalid codec type %s", opt.CodecType)
		_ = conn.Close()
		return
	}
	if err = json.NewEncoder(conn).Encode(opt); err != nil {
		_ = conn.Close()
		return
	}
	client = &Client{
		c:       f(conn),
		seq:     1,
		pending: make(map[uint64]*Call),
	}

	go client.receive()
	return
}

// 连接服务器+创建客户端，封装
func Dial(protocol, addr string, opts ...*Option) (client *Client, err error) {
	conn, err := net.Dial(protocol, addr)
	if err != nil {
		return
	}
	opt := ParseOption(opts...)
	client, err = NewClient(conn, opt)
	return
}

func ParseOption(opts ...*Option) *Option {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption
	}
	return opts[0]
}

// 同步请求
func (c *Client) Call(serviceMethod string, argv, reply interface{}) (err error) {
	call, err := c.Go(serviceMethod, argv, reply)
	if err != nil {
		return
	}

	ctx, _ := context.WithTimeout(context.Background(), timer.ClientCallTimeOut)
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, call.seq)
		c.mu.Unlock()
		return ctx.Err()
	case call := <-call.done:
		return call.Error
	}
}

// 异步请求
func (c *Client) Go(serviceMethod string, argv, reply interface{}) (call *Call, err error) {
	call = &Call{
		serviceMethod: serviceMethod,
		argv:          argv,
		reply:         reply,
		done:          make(chan *Call),
	}

	c.mu.Lock()
	call.seq = c.seq
	c.seq += 1
	c.mu.Unlock()

	err = c.send(call)
	return
}

// 发送请求
func (c *Client) send(call *Call) (err error) {
	// 确保每一条连接，同一时刻只发送一个数据包，防止粘包
	c.sending.Lock()
	defer c.sending.Unlock()

	c.mu.Lock()
	if c.closing {
		c.mu.Unlock()
		err = errors.New("conn is closing")
		return
	}

	c.pending[call.seq] = call
	c.mu.Unlock()

	if err = c.c.Writer(&codec.Header{ServiceMethod: call.serviceMethod, Seq: call.seq, Error: ""}, call.argv); err != nil {
		return
	}
	return
}

// 接收响应
func (c *Client) receive() {
	var err error
	for err == nil {
		var header codec.Header
		if err = c.c.ReadHeader(&header); err != nil {
			break
		}
		c.mu.Lock()
		call := c.pending[header.Seq]
		delete(c.pending, header.Seq)
		c.mu.Unlock()

		switch {
		case call == nil:
			err = c.c.ReadBody(nil)
		case header.Error != "":
			call.Error = fmt.Errorf(header.Error)
			err = c.c.ReadBody(nil)
		default:
			err = c.c.ReadBody(call.reply)
			if err != nil {
				call.Error = errors.New("read body error: " + err.Error())
			}
			call.done <- call
		}
	}
	for _, call := range c.pending {
		call.Error = err
		call.done <- call
	}
	c.Close()
}
