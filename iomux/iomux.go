package iomux

import "golang.org/x/sys/unix"

type IoMuX interface {
	Add(fd int, event unix.EpollEvent) (err error)
	Mod(fd int, event unix.EpollEvent) (err error)
	Remove(fd int, event unix.EpollEvent) (err error)
	Wait() (int, error)
}
