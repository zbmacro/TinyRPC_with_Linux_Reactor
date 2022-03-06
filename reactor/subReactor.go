package reactor

import (
	"TinyRPC/iomux"
	"TinyRPC/network"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"sync"
)

// SubReactor 记录当前reactor监听的文件描述符的相关信息
type SubReactor struct {
	handler *map[int]*connInfo
	mu      sync.RWMutex // 保护handler的修改
	ioMux   iomux.IoMuX  // 操作当前监听文件描述符的实例
}

// 存储每个连接相关的信息
type connInfo struct {
	handlerReadTask *HandlerReadTask // read操作需要的信息
	event           unix.EpollEvent  // 当前监听的事件
	writeWait       chan struct{}    // 通知network层可以write
}

// 子Reactor，监听读事件
func createSubReactor(fdCh chan int, handlerReadTask chan []*HandlerReadTask) {
	var (
		networkWriteWait = make(chan int) // network层的write通知subReactor监听write事件
		handler          = make(map[int]*connInfo)
		subReactor       = &SubReactor{
			handler: &handler,
		}
	)

	ioMux, err := iomux.NewEpoll(5120)
	if err != nil {
		return
	}
	subReactor.ioMux = ioMux

	// 接收mainReactor发送过来的需要处理的文件描述符，以及network层发过来的需要监听write事件的文件描述符
	go func(subReactor *SubReactor) {
		for {
			select {
			case fd := <-fdCh:
				subReactor.mu.Lock()
				wait := make(chan struct{})
				conninfo := &connInfo{
					handlerReadTask: &HandlerReadTask{
						Fd:           fd,
						Conn:         network.NewConnByFd(fd, wait, networkWriteWait),
						SubReactorer: subReactor,
					},
					event: unix.EpollEvent{
						Fd:     int32(fd),
						Events: unix.EPOLLET | unix.EPOLLIN,
					},
					writeWait: wait,
				}
				(*(subReactor.handler))[fd] = conninfo
				subReactor.mu.Unlock()
				if err := subReactor.add(fd); err != nil {
					log.Printf("subReactor add fd %d err:%s\n", fd, err.Error())
				}
			case fd := <-networkWriteWait:
				if err := subReactor.AddWrite(fd); err != nil {
					log.Printf("subReactor add write fd %d err:%s\n", fd, err.Error())
				}
			}
		}
	}(subReactor)

	for {
		nevents, err := ioMux.Wait()
		if err != nil && err != unix.EINTR {
			fmt.Println("subreactor err:", err.Error())
			return
		}

		// 每次收到事件都要创建一个新的HandlerReadTask Slice，否则后面收到的事件可能会覆盖前面的事件
		// 导致handlerRead池处理不到前面的事件，或导致多个handlerRead处理同一Conn导致反序列化数据失败
		var event []*HandlerReadTask
		for ev := 0; ev < nevents; ev++ {
			// 读取文件描述符的相关信息
			subReactor.mu.RLock()
			c, ok := (*(subReactor.handler))[int(ioMux.Events[ev].Fd)]
			subReactor.mu.RUnlock()

			if ioMux.Events[ev].Events&unix.EPOLLIN != 0 {
				if ok {
					event = append(event, c.handlerReadTask)
				}
				// 移除文件描述符的读监听
				_ = subReactor.RemoveRead(int(ioMux.Events[ev].Fd))
			}

			if ioMux.Events[ev].Events&unix.EPOLLOUT != 0 {
				c.writeWait <- struct{}{}
				// 移除文件描述符的写监听
				_ = subReactor.RemoveWrite(int(ioMux.Events[ev].Fd))
			}
		}

		if len(event) > 0 {
			handlerReadTask <- event // 将文件描述符相关信息传递给handler池处理
		}
	}
}

func (sub *SubReactor) add(fd int) (err error) {
	sub.mu.RLock()
	defer sub.mu.RUnlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		return sub.ioMux.Add(fd, conninfo.event)
	}
	return
}

func (sub *SubReactor) AddRead(fd int) (err error) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		if conninfo.event.Events&unix.EPOLLIN == 0 {
			conninfo.event.Events |= unix.EPOLLIN
			(*(sub.handler))[fd] = conninfo
			return sub.ioMux.Mod(fd, conninfo.event)
		}
	}
	return
}

func (sub *SubReactor) AddWrite(fd int) (err error) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		if conninfo.event.Events&unix.EPOLLOUT == 0 {
			conninfo.event.Events |= unix.EPOLLOUT
			(*(sub.handler))[fd] = conninfo
			return sub.ioMux.Mod(fd, conninfo.event)
		}
	}
	return
}

func (sub *SubReactor) RemoveRead(fd int) (err error) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		if conninfo.event.Events&unix.EPOLLIN != 0 {
			conninfo.event.Events ^= unix.EPOLLIN
			(*(sub.handler))[fd] = conninfo
			return sub.ioMux.Mod(fd, conninfo.event)
		}
	}
	return
}

func (sub *SubReactor) RemoveWrite(fd int) (err error) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		if conninfo.event.Events&unix.EPOLLOUT != 0 {
			conninfo.event.Events ^= unix.EPOLLOUT
			(*(sub.handler))[fd] = conninfo
			return sub.ioMux.Mod(fd, conninfo.event)
		}
	}
	return
}

func (sub *SubReactor) Remove(fd int) (err error) {
	sub.mu.Lock()
	defer sub.mu.Unlock()

	conninfo, ok := (*(sub.handler))[fd]
	if ok {
		err = sub.ioMux.Remove(fd, conninfo.event)
		_ = conninfo.handlerReadTask.C.Close()
		delete(*(sub.handler), fd)
	}
	return
}
