package iomux

import (
	"golang.org/x/sys/unix"
)

type Epoll struct {
	Epfd   int // epoll监听的文件描述符
	Events []unix.EpollEvent
}

func (e *Epoll) Add(fd int, event unix.EpollEvent) error {
	return unix.EpollCtl(e.Epfd, unix.EPOLL_CTL_ADD, fd, &event)
}

func (e *Epoll) Mod(fd int, event unix.EpollEvent) error {
	return unix.EpollCtl(e.Epfd, unix.EPOLL_CTL_MOD, fd, &event)
}

func (e *Epoll) Remove(fd int, event unix.EpollEvent) error {
	return unix.EpollCtl(e.Epfd, unix.EPOLL_CTL_DEL, fd, &event)
}

func (e *Epoll) Wait() (int, error) {
	return unix.EpollWait(e.Epfd, e.Events[:], -1)
}

func NewEpoll(size int) (*Epoll, error) {
	epfd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	return &Epoll{
		Epfd:   epfd,
		Events: make([]unix.EpollEvent, size),
	}, nil
}
