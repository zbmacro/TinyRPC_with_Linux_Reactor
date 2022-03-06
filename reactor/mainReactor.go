package reactor

import (
	"TinyRPC/iomux"
	"bufio"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type subReactor struct {
	fd  chan int   // mainReactor向subReactor发送需要监听的fd
	num int        // 当前subReactor监听的fd数量
	mu  sync.Mutex // 保护num
}

// Reactor 对外接口，负责启动mainReactor、subReactor、handler、worker
func Reactor(addr string, s Server) error {
	var (
		subReactors      []*subReactor                   // 记录subReactor的信息
		handlerTask      = make(chan []*HandlerReadTask) // subReactor向handlerRead池发送任务
		workerTask       = make(chan *WorkerTask)        // handler向worker池发送任务
		handlerWriteTask = make(chan *WorkerTask)        // worker池向handlerWriteControl发送响应任务
	)

	fd, err := createTCPSocket(addr)
	if err != nil {
		return err
	}

	// 启动Worker池
	for i := 0; i < 500; i++ {
		go createWorker(workerTask, s, handlerWriteTask)
	}

	// 启动Handler池
	for i := 0; i < 10; i++ {
		go createHandlerRead(workerTask, s, handlerTask) // read池
	}
	go createHandlerWriterControl(s, handlerWriteTask) // writeControl

	// 启动SubReactor
	for i := 0; i < 10; i++ {
		subReactors = append(subReactors, &subReactor{
			fd: make(chan int),
		})
		go createSubReactor(subReactors[i].fd, handlerTask)
	}

	// 启动mainReactor
	return mainReactor(fd, subReactors)
}

func createTCPSocket(addr string) (fd int, err error) {
	// unix.CloseOnExec(fd)和RLock为了在Fork子进程时，关闭子进程文件描述符，防止子进程监听文件描述符。
	syscall.ForkLock.RLock()
	//                         TCP/IP–IPv4      流套接字           TCP协议
	if fd, err = unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP); err == nil {
		unix.CloseOnExec(fd)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return
	}

	// 设置非阻塞模式
	if err = unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return fd, err
	}

	// 获取TCPAddr
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	sa := unix.SockaddrInet4{Port: tcpAddr.Port}
	copy(sa.Addr[:], tcpAddr.IP)

	// 绑定IP和端口
	_ = unix.Bind(fd, &sa)
	err = unix.Listen(fd, maxListenerBacklog())
	return
}

// 全连接队列大小，min(此处返回的值,内核somaxconn)，err为nil时，此处返回的值就是内核somaxconn的值
func maxListenerBacklog() int {
	fd, err := os.Open("/proc/sys/net/core/somaxconn")
	if err != nil {
		return unix.SOMAXCONN
	}
	defer fd.Close()

	rd := bufio.NewReader(fd)
	line, err := rd.ReadString('\n')
	if err != nil {
		return unix.SOMAXCONN
	}

	f := strings.Fields(line)
	if len(f) < 1 {
		return unix.SOMAXCONN
	}

	n, err := strconv.Atoi(f[0])
	if err != nil || n == 0 {
		return unix.SOMAXCONN
	}

	// Linux stores the backlog in a uint16.
	// Truncate number to avoid wrapping.
	// See issue 5030.
	// 65535
	if n > 1<<16-1 {
		n = 1<<16 - 1
	}

	return n
}

// 主Reactor，监听accept事件
func mainReactor(fd int, subReactors []*subReactor) error {
	ioMux, err := iomux.NewEpoll(1)
	if err != nil {
		return err
	}

	var event unix.EpollEvent
	event.Fd = int32(fd)
	event.Events = unix.EPOLLIN
	if err := ioMux.Add(fd, event); err != nil {
		return err
	}

	for {
		_, err := ioMux.Wait()
		if err != nil && err != unix.EINTR {
			return err
		}
		connfd, _, err := unix.Accept(fd)
		if err != nil {
			continue
		}
		// 设置非阻塞模式
		if err = unix.SetNonblock(connfd, true); err != nil {
			_ = unix.Close(connfd)
			continue
		}

		// 此处存在脏读问题，负载均衡允许细微的误差
		var min int
		for i := 1; i < len(subReactors); i++ {
			if subReactors[i].num < subReactors[min].num {
				min = i
			}
		}
		subReactors[min].mu.Lock()
		subReactors[min].num++
		subReactors[min].mu.Unlock()
		subReactors[min].fd <- connfd // 将连接的读写事件交给subReactor处理
	}
}
