package register

import (
	"TinyRPC/client"
	"TinyRPC/config"
	"errors"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// 负载均衡

type Balance struct {
	services   GetInfo             // 服务列表
	lastUpdate time.Time           // 服务列表最后更新时间
	r          *rand.Rand          // 随机数实例，使用时间戳设置随机数种子，避免每次产生相同的随机数序列
	index      map[ServiceName]int // RR轮询的位置
	mu         sync.Mutex
	client     *client.Client // 复用与register的连接
}

type SelectMode int // 负载方式

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

// NewBalance 返回负载均衡实例
func NewBalance() (*Balance, error) {
	balance := &Balance{
		services: make(GetInfo),
		r:        rand.New(rand.NewSource(time.Now().UnixNano())),
		index:    make(map[ServiceName]int),
	}
	c, err := client.Dial("tcp", config.RegisterAddr)
	if err != nil {
		return nil, errors.New("refresh services from register error:" + err.Error())
	}
	balance.client = c
	return balance, nil
}

// Refresh 向注册中心获取可用服务器列表
func (b *Balance) Refresh() error {
	if (config.BalanceServices == 0 && len(b.services) != 0) || b.lastUpdate.Add(config.BalanceServices).After(time.Now()) {
		return nil
	}

	// 确保不会重复向注册中心请求服务列表
	b.mu.Lock()
	defer b.mu.Unlock()
	if (config.BalanceServices == 0 && len(b.services) != 0) || b.lastUpdate.Add(config.BalanceServices).After(time.Now()) {
		return nil
	}

	log.Println("refresh services from register")
	if err := b.client.Call("Register.Get", struct{}{}, &b.services); err != nil {
		return errors.New("refresh services from register error:" + err.Error())
	}

	for key, _ := range b.services {
		b.index[key] = b.r.Intn(math.MaxInt32) // 每个服务随机一个轮询开始index，防止不同服务也请求同一服务器
	}

	if len(b.services) > 0 {
		b.lastUpdate = time.Now()
	}
	return nil
}

func (b *Balance) Get(mode SelectMode, serviceName ServiceName) (addr Addr, err error) {
	if err = b.Refresh(); err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	addrs, ok := b.services[serviceName]
	n := len(addrs)
	if !ok || n == 0 {
		err = errors.New("services is nil")
		return
	}

	switch mode {
	case RandomSelect:
		t := b.r.Intn(n)
		addr = addrs[t]
	case RoundRobinSelect:
		addr = addrs[b.index[serviceName]%n]
		b.index[serviceName] = (b.index[serviceName] + 1) % n
	}
	return
}
