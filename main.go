package main

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
)

var (
	// proxy config
	TunPort int
	TcpPort int

	// bridge config
	ServerAddr string
	TargetAddr string
)

func init() {
	// 读取环境变量
	if envPort := os.Getenv("TUNPORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			TunPort = port
		} else {
			log.Printf("环境变量TUNPORT格式错误: %v", err)
		}
	}
	if envPort := os.Getenv("TCPPORT"); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil {
			TcpPort = port
		} else {
			log.Printf("环境变量TCPPORT格式错误: %v", err)
		}
	}
	// TunPort 和 TcpPort 只设置其中一个是无法起到效果的
	if TunPort == 0 && TcpPort == 0 {
		log.Printf("TunPort 和 TcpPort 只设置其中一个是无法起到效果的")
	}
	if TunPort != 0 && TcpPort != 0 {
		log.Printf("TunPort 和 TcpPort 只设置其中一个是无法起到效果的")
	}

	ServerAddr = os.Getenv("SERVER")
	TargetAddr = os.Getenv("TARGET")
	// ServerAddr和TargetAddr如果只设置一个 也无法让bridge正常工作
	if ServerAddr != "" && TargetAddr == "" {
		log.Printf("ServerAddr和TargetAddr如果只设置一个 无法让bridge正常工作")
	}
	if ServerAddr == "" && TargetAddr != "" {
		log.Printf("ServerAddr和TargetAddr如果只设置一个 无法让bridge正常工作")
	}

	// 打印所有的变量
	log.Printf("TunPort: %d, TcpPort: %d, ServerAddr: %s, TargetAddr: %s", TunPort, TcpPort, ServerAddr, TargetAddr)
}

func main() {
	// 设置单线程
	runtime.GOMAXPROCS(1)
	if TunPort != 0 && TcpPort != 0 {
		log.Printf("启动proxy模式")
		proxy := NewProxy(TunPort, TcpPort)
		proxy.StartProxy()
	} else if ServerAddr != "" && TargetAddr != "" {
		log.Printf("启动bridge模式")
		bridge := NewBridge(ServerAddr, TargetAddr)
		bridge.StartBridge()
	} else {
		// 环境变量设置错误， 并没有成功启动任何模式
		log.Printf("环境变量设置错误， 并没有成功启动任何模式 程序在10秒后退出")
		// 打印： 设置TUNPORT和TCPPORT 为 proxy模式
		log.Printf("设置TUNPORT和TCPPORT 为 proxy模式")
		// 打印： 设置SERVER和TARGET 为 bridge模式
		log.Printf("设置SERVER和TARGET 为 bridge模式")
		time.Sleep(10 * time.Second)
		os.Exit(1)
	}
	for {
		time.Sleep(1 * time.Second)
	}
}
