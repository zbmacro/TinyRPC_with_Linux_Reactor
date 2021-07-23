package register

import (
	"RPC"
	"RPC/timer"
	"errors"
	"log"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// 负载均衡
type Balance struct {
	services     []string
	registerAddr string
	duration     time.Duration
	lastUpdate   time.Time  // 服务列表最后更新时间
	r            *rand.Rand // 随机数实例，使用时间戳设置随机数种子，避免每次产生相同的随机数序列
	index        int        // RR轮询的位置
	mu           sync.Mutex
	client       *RPC.Client // 复用与register的连接
}

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
)

func NewBalance(registerAddr string) (*Balance, error) {
	balance := &Balance{
		registerAddr: registerAddr,
		duration:     timer.BalanceServices,
		r:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	balance.index = balance.r.Intn(math.MaxInt32)
	client, err := RPC.Dial("tcp", registerAddr)
	if err != nil {
		return nil, errors.New("refresh services from register error:" + err.Error())
	}
	balance.client = client
	return balance, nil
}

// 向注册中心获取可用服务器列表
func (b *Balance) Refresh() error {
	// 确保不会重复向注册中心请求服务列表
	b.mu.Lock()
	defer b.mu.Unlock()
	if (b.duration == 0 && len(b.services) != 0) || b.lastUpdate.Add(b.duration).After(time.Now()) {
		return nil
	}

	log.Println("refresh services from register")
	var services Services
	if err := b.client.Call("Register.Get", struct{}{}, &services); err != nil {
		return errors.New("refresh services from register error:" + err.Error())
	}

	n := len(services)
	b.services = make([]string, 0, n)
	for _, service := range services {
		if strings.TrimSpace(service) != "" {
			b.services = append(b.services, strings.TrimSpace(service))
		}
	}
	if n > 0 {
		b.lastUpdate = time.Now()
	}
	return nil
}

func (b *Balance) Get(mode SelectMode) (addr string, err error) {
	if err = b.Refresh(); err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(b.services)
	if n == 0 {
		err = errors.New("services is nil")
		return
	}
	switch mode {
	case RandomSelect:
		addr = b.services[b.r.Intn(n)]
	case RoundRobinSelect:
		addr = b.services[b.index%n]
		b.index = (b.index + 1) % n
	}
	return
}

func (b *Balance) GetAll() (services []string, err error) {
	if err = b.Refresh(); err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	copy(services, b.services)
	return
}
