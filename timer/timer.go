package timer

import "time"

// 超时时间控制
const (
	RegisterService time.Duration = time.Minute * 2               // 注册中心 服务器 过期时间，0：无限期、默认值：2m
	SendHeartbeat   time.Duration = RegisterService - time.Minute // 发送心跳时间间隔，默认值：过期时间-1m
	BalanceServices time.Duration = time.Second * 25              // 负载均衡 服务列表 过期时间，0：无限期、默认值：25s

	ConnectTimeOut       time.Duration = time.Second * 10 // 建立连接超时时间，0：无限期、默认值：10s
	ClientCallTimeOut    time.Duration = time.Second * 10 // 客户端调用超时时间，0：无限期、默认值：10s
	ServiceHandleTimeOut time.Duration = time.Second * 10 // 服务端处理超时时间，0：无限期、默认值：10s
)
