package register

import (
	"TinyRPC/client"
	"TinyRPC/config"
	"TinyRPC/server"
	"errors"
	"log"
	"sync"
	"time"
)

// RPC接口

type Addr string        // 服务器地址
type ServiceName string // 服务名称

// PostInfo 服务端提供的服务列表以及服务器地址，用于注册到注册中心
type PostInfo struct {
	Address      Addr
	ServicesName []ServiceName
}

// GetInfo 客户端获取服务列表
type GetInfo map[ServiceName][]Addr

// Post 服务端注册服务列表
func (r *Register) Post(postInfo PostInfo, reply *struct{}) error {
	r.addServer(&postInfo)
	return nil
}

// Put 接收服务端心跳
func (r *Register) Put(addr Addr, reply *struct{}) error {
	return r.putServer(&addr)
}

// Get 客户端获取服务列表
func (r *Register) Get(args struct{}, getInfo *GetInfo) error {
	*getInfo = r.aliveServer()
	return nil
}

// 注册中心服务端

// NewRegister 返回注册中心实例
func NewRegister(s *server.Server) {
	register := &Register{
		services: make(map[Addr]*serviceList),
		timeout:  config.RegisterService,
	}
	s.Register(register)
}

// Register 注册中心实例保存的相关信息
type Register struct {
	services   map[Addr]*serviceList // map[服务器地址]服务列表
	servicesMu sync.Mutex
	timeout    time.Duration // 服务器过期时间，0为无限期
}

type serviceList struct {
	servicesName *[]ServiceName // 提供的服务列表
	heartbeat    time.Time      // 心跳时间
}

// 服务端注册服务列表
func (r *Register) addServer(postInfo *PostInfo) {
	r.servicesMu.Lock()
	servicelist, ok := r.services[postInfo.Address]
	if !ok {
		servicelist = &serviceList{}
		r.services[postInfo.Address] = servicelist
	}
	r.servicesMu.Unlock()

	servicelist.servicesName = &postInfo.ServicesName
	servicelist.heartbeat = time.Now()
}

// 更新服务器心跳时间
func (r *Register) putServer(addr *Addr) error {
	r.servicesMu.Lock()
	servicelist, ok := r.services[*addr]
	if !ok {
		return errors.New("please call Post to register")
	}
	r.servicesMu.Unlock()

	servicelist.heartbeat = time.Now()
	return nil
}

// 读取所有可用的服务器
func (r *Register) aliveServer() (getInfo GetInfo) {
	r.servicesMu.Lock()
	defer r.servicesMu.Unlock()

	getInfo = make(map[ServiceName][]Addr)
	for addr, servicelist := range r.services {
		// 判断过期时间
		if r.timeout == 0 || servicelist.heartbeat.Add(r.timeout).After(time.Now()) {
			for _, servername := range *servicelist.servicesName {
				addrs, ok := getInfo[servername]
				if !ok {
					addrslice := make([]Addr, 0, 1)
					addrs = addrslice
				}
				addrs = append(addrs, addr)
				getInfo[servername] = addrs
			}
		}
	}
	return
}

// 注册中心客户端（定时发送心跳封装，程序服务端用于发送心跳）

// Heartbeat 确保同一台机器只有一条连接与注册中心相连
type Heartbeat struct {
	Addr   string
	Client *client.Client
}

// SendServices 注册服务列表到注册中心
func (h *Heartbeat) SendServices(postInfo PostInfo) error {
	var reply struct{}
	return h.Client.Call("Register.Post", postInfo, &reply)
}

// SendHeartbeat 发送心跳
func (h *Heartbeat) SendHeartbeat() {
	var err error
	var reply struct{}
	t := time.NewTicker(config.SendHeartbeat)

	defer t.Stop()
	defer h.Client.Close()

	for !h.Client.IsClose() {
		<-t.C
		if err = h.Client.Call("Register.Put", h.Addr, &reply); err != nil {
			log.Printf("rpc client: send heartbeat to register error: %s\n", err.Error())
		}
	}
}
