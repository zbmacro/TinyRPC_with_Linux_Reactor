# 项目简介
本项目是使用GO语言开发的基于Linux的轻量级RPC框架，包括服务端与客户端两部分，同时实现了注册中心与负载均衡，项目旨在保证响应速度的同时提高单系统的吞吐量。

# 运行环境
* 系统：CentOS 7.9.2009
* Golang版本：go 17.7 linux/amd64

# 例子
### 服务端
```go
package main

import "TinyRPC"

type Compute struct{}
type Args struct {
  Num1, Num2 int
}

func (c Compute) Sum(args Args, reply *int) error {
  *reply = args.Num1 + args.Num1
  return nil
}

func main() {
  addr := "172.17.0.2"
  server := TinyRPC.NewServer(addr) // 启动
  server.Register(new(Foo)) // 注册服务
  
  // 阻塞main协程
  var ch chan struct{}
  <-ch
}
```
### 注册中心+服务端
注册中心
```go
//config/config.go
RegisterAddr    string        = "172.17.0.2:9999"
```
```go
package main

import "TinyRPC"

func main() {
  TinyRPC.NewRegister() //启动注册中心
  
  // 阻塞main协程
  var ch chan struct{}
  <-ch
}
```
服务端
```go
package main

import "TinyRPC"

type Compute struct{}
type Args struct {
  Num1, Num2 int
}

func (c Compute) Sum(args Args, reply *int) error {
  *reply = args.Num1 + args.Num1
  return nil
}

func main() {
  addr := "172.17.0.2"
  server := TinyRPC.NewServer(addr) // 启动
  server.Register(new(Foo)) // 注册服务
    
  // 启动注册中心客户端，发送服务列表及定时发送心跳
  if err := TinyRPC.ServerStartRegisterClient(addr, server); err != nil {
    log.Println(err.Error())
  }
  
  // 阻塞main协程
  var ch chan struct{}
  <-ch
}
```

### 客户端
```go
package main

import (
  "TinyRPC"
  "TinyRPC/server"
  "fmt"
)

type Args struct {
  Num1, Num2 int
}

func main(){
  addr := "172.17.0.2:9991"
  //创建客户端并向服务端发起连接
  c, err := TinyRPC.NewClient(addr, &server.Option{CodecType: "gob"})
  if err != nil {
    fmt.Printf("rpc client: %s\n", err.Error())
    return
  }

  // 请求Foo.Sum服务
  var reply int
  if err := c.Call("Foo.Sum", Args{Num1: 1, Num2: 2}, &reply); err != nil {
    fmt.Println("rpc client: call error: ", err.Error())
    return
  }
  fmt.Printf("res %d", reply)
}
```

### 负载均衡+客户端（需启动注册中心）

```go
//config/config.go
RegisterAddr    string        = "172.17.0.2:9999"
```

```go
package main

import (
  "TinyRPC"
  "TinyRPC/register"
  "TinyRPC/server"
  "fmt"
)

type Args struct {
  Num1, Num2 int
}

func main() {
  // 创建客户端并向注册中心发起连接
  c, err := TinyRPC.NewClientByBalance(register.RandomSelect, &server.Option{CodecType: "gob"})
  if err != nil {
    fmt.Printf("rpc client: %s\n", err.Error())
    return
  }

  // 请求Foo.Sum服务
  var reply int
  if err := c.Call("Foo.Sum", Args{Num1: 1, Num2: 2}, &reply); err != nil {
    fmt.Println("rpc client: call error: ", err.Error())
    return
  }
  fmt.Printf("res %d", reply)
}
```
# 架构
![RPC项目架构](https://raw.githubusercontent.com/zbmacro/TinyRPC_with_Linux_Reactor/master/架构图.svg)

# 性能测试

测试结果显示，本项目在处理速度相差不大的情况下，内存占用方面优于go标准rpc框架，且随着连接数量的增长效果越明显。本测试仅针对单服务端进行，并未启动注册中心。

### 测试环境
* **CPU:** Intel(R) Core(TM) i5-9300H CPU @ 2.40GHz, 6核
* **内存:** 12G
* **Go:** 17.7 linux/amd64
* **OS:** CentOS 7.9.2009
* **Docker:** alpine 3.9

### 测试方法
* 服务端与客户端位于同一台机器的不同docker容器中。
* 一个docker容器启动服务端。
* 内存测试：通过docker stats观察服务端的内存占用情况。
  - 10万连接。5个docker启动客户端，每个客户端发起2万连接。
  - 20万连接。10个docker启动客户端，每个客户端发起2万连接。
  - 26万连接。12个docker启动客户端，每个客户端发起2万连接。
* 处理速度测试：程序输出执行的时间长度。
  - 10万连接，200万业务请求。10个docker启动客户端，每个客户端发起2万连接，每条连接发起10次业务请求。

### 测试结果
##### 内存测试。
10万连接（进行四次测试），本项目比GoRPC内存占用**降低20.9%**。  

| 框架       | 1      | 2      | 3      | 4      | 平均值       |
| ---------- | ------ | ------ | ------ | ------ | ------------ |
| **本项目** | 1.060G | 1.014G | 1.070G | 1.051G | **1.04875G** |
| **Go RPC** | 1.321G | 1.336G | 1.331G | 1.321G | **1.32725G** |

20万连接（进行四次测试），本项目比GoRPC内存占用**降低21.3%**。  

| 框架       | 1      | 2      | 3      | 4      | 平均值       |
| ---------- | ------ | ------ | ------ | ------ | ------------ |
| **本项目** | 1.941G | 1.915G | 1.893G | 1.888G | **1.90925G** |
| **Go RPC** | 2.475G | 2.451G | 2.409G | 2.373G | **2.42700G** |

26万连接（进行四次测试），本项目比GoRPC内存占用**降低22.1%**。  

| 框架   | 1      | 2      | 3      | 4      | 平均值       |
| ------ | ------ | ------ | ------ | ------ | ------------ |
| 本项目 | 2.689G | 2.866G | 2.907G | 2.881G | **2.83575G** |
| Go RPC | 3.646G | 3.644G | 3.644G | 3.627G | **3.64025G** |

##### 处理速度测试

10万连接，200万请求（进行5次测试），本项目比GoRPC在处理200万次业务请求中慢了2s。

本框架：

| 客户端       | 1         | 2         | 3         | 4        | 5         | 总平均时间    |
| ------------ | --------- | --------- | --------- | -------- | --------- | ------------- |
| **client1**  | 25232ms   | 25615ms   | 46152ms   | 18623ms  | 63561ms   |               |
| **client2**  | 55359ms   | 68442ms   | 19439ms   | 67766ms  | 35914ms   |               |
| **client3**  | 66877ms   | 56440ms   | 36787ms   | 59424ms  | 50311ms   |               |
| **client4**  | 66063ms   | 16556ms   | 49512ms   | 36151ms  | 55040ms   |               |
| **client5**  | 28941ms   | 39510ms   | 63200ms   | 55201ms  | 62320ms   |               |
| **client6**  | 46505ms   | 42771ms   | 30738ms   | 44235ms  | 41892ms   |               |
| **client7**  | 35116ms   | 32801ms   | 24912ms   | 39257ms  | 28254ms   |               |
| **client8**  | 50951ms   | 54741ms   | 58282ms   | 49966ms  | 22284ms   |               |
| **client9**  | 57417ms   | 56361ms   | 53054ms   | 73344ms  | 42088ms   |               |
| **client10** | 40406ms   | 65820ms   | 43364ms   | 69421ms  | 21424ms   |               |
| **平均时间** | 47286.7ms | 45905.7ms | 42544.0ms | 5138.8ms | 42308.8ms | **45876.8ms** |

GoRPC：

| 客户端       | 1         | 2         | 3         | 4         | 5         | 总平均时间    |
| ------------ | --------- | --------- | --------- | --------- | --------- | ------------- |
| **client1**  | 65400ms   | 39294ms   | 53039ms   | 30554ms   | 45157ms   |               |
| **client2**  | 52970ms   | 27638ms   | 21094ms   | 64909ms   | 50772ms   |               |
| **client3**  | 36557ms   | 22526ms   | 59155ms   | 14279ms   | 66735ms   |               |
| **client4**  | 28397ms   | 47451ms   | 52055ms   | 40831ms   | 34680ms   |               |
| **client5**  | 44267ms   | 60755ms   | 53867ms   | 47342ms   | 53871ms   |               |
| **client6**  | 67999ms   | 42026ms   | 34216ms   | 61226ms   | 39073ms   |               |
| **client7**  | 39229ms   | 55154ms   | 40635ms   | 20721ms   | 14412ms   |               |
| **client8**  | 59179ms   | 32866ms   | 25487ms   | 35522ms   | 26122ms   |               |
| **client9**  | 19330ms   | 63199ms   | 33349ms   | 60438ms   | 59930ms   |               |
| **client10** | 54276ms   | 50143ms   | 62002ms   | 45893ms   | 21588ms   |               |
| **平均时间** | 46760.4ms | 44105.2ms | 43489.9ms | 42171.5ms | 41234.0ms | **43552.2ms** |

# 参考

* GoRPC