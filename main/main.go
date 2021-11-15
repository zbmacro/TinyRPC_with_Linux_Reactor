package main

import (
	"RPC"
	"RPC/register"
	"RPC/xclient"
	"log"
	"net"
	"sync"
	"time"
)

type Foo struct{}
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	//time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num1
	return nil
}

// 启动注册中心
const RegisterAddr string = ":9999"

func startRegister() {
	l, err := net.Listen("tcp", RegisterAddr)
	if err != nil {
		log.Fatal("server network error: ", err)
	}
	server := RPC.NewServer(l)
	server.Register(register.NewRegister()) // 注册Get和Post方法，分别用于获取可用服务列表和接收心跳
	log.Println("register server start at ", l.Addr())
}

// 启动服务端
func startServer(wg *sync.WaitGroup, addr string) {
	wg.Add(1)
	defer wg.Done()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal("server network error: ", err)
	}
	server := RPC.NewServer(l)
	server.Register(new(Foo))
	register.DefaultHeartbeat.SendHeartbeat(RegisterAddr, "tcp@"+l.Addr().String(), 0)
	log.Println("rpc server start at ", l.Addr())
}

func main() {
	startRegister()
	time.Sleep(time.Second)

	var wg sync.WaitGroup
	go startServer(&wg, ":9991")
	go startServer(&wg, ":9992")
	go startServer(&wg, ":9993")
	go startServer(&wg, ":9994")
	go startServer(&wg, ":9995")
	go startServer(&wg, ":9996")
	go startServer(&wg, ":9997")
	wg.Wait()

	client, err := xclient.Dial(RegisterAddr, register.RandomSelect, &RPC.Option{CodecType: "json"})
	if err != nil {
		log.Fatalf("rpc client: %s", err.Error())
	}

	for i := 1; i < 500000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var reply int
			if err := client.Call("Foo.Sum", Args{Num1: i, Num2: i}, &reply); err != nil {
				log.Println("rpc client: call error: ", err.Error(), i)
				return
			}
		}(i)
	}
	wg.Wait()
}
