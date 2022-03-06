package network

import (
	"errors"
	"golang.org/x/sys/unix"
	"net"
	"runtime"
)

// 封装读写，解决粘包问题

type Conn struct {
	Fd        int
	conn      net.Conn
	isFd      bool          // 记录是用fd还是net.conn创建的network实例
	noFrist   bool          // 判断是不是第一个报文，第一个报文是协商数据，需要解决粘包获取。客户端第一个发送的是协商报文、服务端第一个收到的是协商报文
	head      uint8         // 读取到的协商报文首部长度
	writeChan chan int      // 通知subReactor监听write事件
	wait      chan struct{} // 等待subReactor通知文件描述符可写
}

// NewConnByFd 匹配服务端
func NewConnByFd(fd int, wait chan struct{}, writeChan chan int) *Conn {
	c := newConn()
	c.Fd = fd
	c.isFd = true
	c.writeChan = writeChan
	c.wait = wait
	return c
}

// NewConnByConn 匹配客户端
func NewConnByConn(conn net.Conn) *Conn {
	c := newConn()
	c.conn = conn
	c.isFd = false
	return c
}

func newConn() *Conn {
	return &Conn{}
}

func (c *Conn) Read(b []byte) (n int, err error) {
	if !c.noFrist {
		// 第一个报文，协商报文，需要解决粘包
		if c.head == 0 {
			head := make([]byte, 1)
			n, err = c.read(head)
			if n == 1 {
				c.head = head[0]
			}
		}

		if err == nil {
			// 读取协商报文体
			t := cap(b)
			if t > int(c.head) {
				t = int(c.head)
			}

			n, err = c.read(b[:t])
			if err == nil {
				if uint8(n) == c.head {
					c.noFrist = true
				} else {
					c.head -= uint8(n)
				}
			}
		}
	} else {
		n, err = c.read(b)
	}

	if n == 0 {
		return n, errors.New("close")
	} else if n == -1 {
		return 0, err
	}
	return
}

func (c *Conn) read(b []byte) (n int, err error) {
	if c.isFd {
		n, err = unix.Read(c.Fd, b)
	} else {
		n, err = c.conn.Read(b)
	}

	runtime.KeepAlive(c.Fd)
	return
}

func (c *Conn) Write(b []byte) (n int, err error) {
	if !c.noFrist {
		head := make([]byte, 1)
		head[0] = byte(len(b))
		n, err = c.write(head)
		if n == 1 {
			var t int
			for err == nil && t != len(b) {
				n, err = c.write(b)
				t += n
			}
			c.noFrist = true
		}
	} else {
		n, err = c.write(b)
	}

	return
}

func (c *Conn) write(b []byte) (n int, err error) {
	if c.isFd {
		var tmp int
		for n < len(b) {
			tmp, err = unix.Write(c.Fd, b[n:])
			if tmp > 0 {
				n += tmp
			}
			if n < len(b) {
				c.writeChan <- c.Fd
				_ = <-c.wait
			}
		}
	} else {
		n, err = c.conn.Write(b)
	}

	runtime.KeepAlive(c.Fd)
	return
}

func (c *Conn) Close() error {
	if c.isFd {
		return unix.Close(c.Fd)
	} else {
		return c.conn.Close()
	}
}
