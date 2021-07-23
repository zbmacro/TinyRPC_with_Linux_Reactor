package main

import (
	"RPC"
	"RPC/register"
	"RPC/xclient"
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
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
	// log.SetFlags(0)

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
	go startServer(&wg, ":9998")
	wg.Wait()

	client, err := xclient.Dial(RegisterAddr, register.RandomSelect, &RPC.Option{CodecType: "gob"})
	if err != nil {
		log.Fatalf("rpc client: %s", err.Error())
	}
	time.Sleep(time.Second) // Dial会想服务端发送Option，减少与Call发生粘包的概率

	scanner := bufio.NewScanner(os.Stdin)
	t := time.Now().UnixNano()
	for scanner.Scan() {
		switch scanner.Text() {
		case "q":
			break
		case "r":
			for i := 1; i < 5; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					var reply int
					if err := client.Call("Foo.Sum", Args{Num1: i, Num2: i * i}, &reply); err != nil {
						log.Println("rpc client: call error: ", err.Error())
						return
					}
					fmt.Printf("Foo.Sum %d + %d = %d\n", i, i*i, reply)
				}(i)
			}
		case "t":
			fmt.Println("time = ", (time.Now().UnixNano()-t)/1e9)
		}
	}
	wg.Wait()
}
