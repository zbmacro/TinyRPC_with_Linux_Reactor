package reactor

import (
	"golang.org/x/sys/unix"
	"time"
)

// handlerRead池，处理读事件
func createHandlerRead(workerTask chan *WorkerTask, s Server, handlerReadTask chan []*HandlerReadTask) {
	for {
		event := <-handlerReadTask
		for i := 0; i < len(event); i++ {
			var err error
			var req *Request
			t := event[i]

			for {
				// 序列化方式为空时，需要先确认序列化方式
				if t.C == nil {
					t.CMu.Lock()
					if t.C == nil {
						err = s.SelectCodec(t)
					}
					t.CMu.Unlock()
				} else {
					req, err = s.ServerCodec(t)          // 反序列化数据
					if err == nil || req.H.Error != "" { // 没有报错或header.Error有错误信息都需要处理，header.Error不为空将直接发送给用户
						handle := &WorkerTask{
							Fd:           t.Fd,
							C:            t.C,
							Req:          req,
							Sending:      &t.Sending,
							SubReactorer: t.SubReactorer,
						}
						dispatchRead(handle, workerTask)
					}
				}

				if err == nil {
					continue
				} else if err == unix.EAGAIN { // 此错误为数据还未到达缓冲区，非致命错误，将描述符重新加入ioMux中监听
					err = t.SubReactorer.AddRead(t.Fd)
					if err != nil {
						_ = t.SubReactorer.Remove(t.Fd)
					}
					break
				} else { // 错误可能由 1.套接字已关闭 2.数据错误无法反序列化等，此时服务端要关闭套接字
					_ = t.SubReactorer.Remove(t.Fd)
					break
				}
			}
		}
	}
}

// 读任务分发
func dispatchRead(work *WorkerTask, workerTask chan *WorkerTask) {
	workerTask <- work // 将反序列化好的数据发送给worker池执行业务逻辑
}

// 最大程度利用已有write goroutine，同时保证worker不阻塞
func createHandlerWriterControl(s Server, handlerWriteTask chan *WorkerTask) {
	writeTaskChan := make(chan *WorkerTask)
	for {
		write := <-handlerWriteTask
	wait:
		for {
			select {
			case writeTaskChan <- write:
				break wait
			default:
				go createHandlerWriter(s, writeTaskChan)
			}
		}
	}
}

// 实际执行write的goroutine，经过了duration没有收到任务退出goroutine
func createHandlerWriter(s Server, handlerWriteTask chan *WorkerTask) {
	var write *WorkerTask
	duration := 60 * time.Second
	timer := time.NewTimer(duration)

	for {
		timer.Reset(duration)
		select {
		case write = <-handlerWriteTask:
		case <-timer.C:
			return
		}

		write.Sending.Lock()
		err := s.SendResponse(write)
		if err != nil { // 写错误
			_ = write.SubReactorer.Remove(write.Fd)
		}
		write.Sending.Unlock()
	}
}
