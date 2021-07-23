package register

import (
	"RPC"
	"RPC/timer"
	"log"
	"sync"
	"time"
)

type ServerItem struct {
	Addr  string
	start time.Time
}

type Register struct {
	services map[string]*ServerItem
	timeout  time.Duration // 服务器过期时间，0为无限期
	mu       sync.Mutex
}

func NewRegister() *Register {
	return &Register{
		services: make(map[string]*ServerItem),
		timeout:  timer.RegisterService,
	}
}

// 添加服务器或更新服务器时间（心跳）
func (r *Register) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.services[addr]; !ok {
		r.services[addr] = &ServerItem{Addr: addr}
	}
	r.services[addr].start = time.Now()
}

// 读取所有可用的服务器
func (r *Register) aliveServer() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var services []string
	for _, service := range r.services {
		// 判断start+r.timeout时候在time.Now()之后
		if r.timeout == 0 || service.start.Add(r.timeout).After(time.Now()) {
			services = append(services, service.Addr)
		} else {
			delete(r.services, service.Addr)
		}
	}
	return services
}

// 确保同一台机器只有一条连接与注册中心相连
type Heartbeat struct {
	client *RPC.Client
	mu     sync.Mutex
}

var DefaultHeartbeat = &Heartbeat{}

// 心跳, duration 发送心跳的间隔，默认比过期时间少1分钟
func (h *Heartbeat) SendHeartbeat(registryAddr, addr string, duration time.Duration) {
	if duration == 0 {
		duration = timer.SendHeartbeat
	}
	var err error
	var reply struct{}

	h.mu.Lock()
	defer h.mu.Unlock()
	client := h.client
	if client == nil {
		client, err = RPC.Dial("tcp", registryAddr)
		if err != nil {
			log.Printf("rpc client: connect registry %s->%s error: %s\n", addr, registryAddr, err.Error())
			return
		}
		h.client = client
	}

	if err = client.Call("Register.Post", addr, &reply); err != nil {
		log.Printf("rpc client: send heartbeat to register %s->%s error: %s\n", addr, registryAddr, err.Error())
		return
	} else {
		log.Printf("rpc client: send heartbeat to register %s->%s \n", addr, registryAddr)
	}

	go func() {
		t := time.NewTicker(duration)
		defer t.Stop()
		for err == nil {
			<-t.C
			if err = client.Call("Register.Post", addr, &reply); err != nil {
				log.Printf("rpc client: send heartbeat to register %s->%s error: %s\n", addr, registryAddr, err.Error())
				break
			} else {
				log.Printf("rpc client: send heartbeat to register %s->%s \n", addr, registryAddr)
			}
		}
	}()
}

// RPC接口
type Services []string

func (r *Register) Get(args struct{}, reply *Services) error {
	*reply = r.aliveServer()
	return nil
}

func (r *Register) Post(args string, reply *struct{}) error {
	r.putServer(args)
	return nil
}
