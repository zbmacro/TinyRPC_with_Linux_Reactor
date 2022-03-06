package client

import (
	"TinyRPC/codec"
	"TinyRPC/network"
	"TinyRPC/server"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
)

// Client 客户端相关信息
type Client struct {
	c       codec.Codec      // 序列化方式
	seq     uint64           // 最新的请求序号
	pending map[uint64]*Call // 已发送但未收到响应的请求
	mu      sync.Mutex       // 确保seq的并发安全
	sending sync.Mutex       // 请求的报文必须逐个发送
	closing bool             // 客户端关闭标识
}

// Call 请求相关信息
type Call struct {
	serviceMethod string      // 请求服务信息
	seq           uint64      // 请求序号
	argv, reply   interface{} //参数
	Error         error       // 服务端返回的错误信息
	done          chan *Call  // 通知请求的响应已收到
}

// DefaultOption 默认协商信息
var DefaultOption = &server.Option{
	CodecType: "gob",
}

// Dial 连接服务器+创建客户端
func Dial(protocol, addr string, opts ...*server.Option) (client *Client, err error) {
	conn, err := net.Dial(protocol, addr)
	if err != nil {
		return
	}
	opt := ParseOption(opts...)
	client, err = NewClient(conn, opt)
	return
}

func ParseOption(opts ...*server.Option) *server.Option {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption
	}
	return opts[0]
}

// NewClient 创建客户端
func NewClient(conn net.Conn, opt *server.Option) (client *Client, err error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err = fmt.Errorf("invalid codec type %s", opt.CodecType)
		_ = conn.Close()
		return
	}

	client = &Client{
		seq:     1,
		pending: make(map[uint64]*Call),
	}
	c := network.NewConnByConn(conn) // 自定义了数据包的格式

	// 发送协商报文
	client.sending.Lock()
	defer client.sending.Unlock()
	if err = json.NewEncoder(c).Encode(opt); err != nil {
		_ = conn.Close()
		return
	}

	client.c = f(c)
	go client.receive()
	return
}

// Call 同步请求，调用异步请求并等到call.done通知
func (c *Client) Call(serviceMethod string, argv, reply interface{}) (err error) {
	call, err := c.Go(serviceMethod, argv, reply)
	caller := <-call.done
	return caller.Error
}

// Go 异步请求
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
	// 确保每一条连接，同一时刻只发送一个数据包，数据包混乱
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
		log.Println("client send request error: ", err)
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
			if err.Error() != "close" {
				fmt.Println("client read header err:", err)
			}
			break
		}

		c.mu.Lock()
		call := c.pending[header.Seq]
		delete(c.pending, header.Seq)
		c.mu.Unlock()

		switch {
		case call == nil:
			err = c.c.ReadBody(nil)
			fmt.Println("call is nil", header)
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
	c.mu.Lock()
	for _, call := range c.pending {
		call.Error = err
		select {
		case call.done <- call:
			continue
		default:
			continue
		}
	}
	c.mu.Unlock()
	c.Close()
}

// Close 关闭连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closing {
		c.closing = true
		_ = c.c.Close()
	}
}

func (c *Client) IsClose() bool {
	return c.closing
}
