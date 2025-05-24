package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

// Proxy QUIC代理服务器
type Proxy struct {
	listener        *quic.Listener
	conn            quic.Connection
	heartbeatStream quic.Stream
	lastHeartbeat   time.Time

	tcpport     int
	quicport    int
	tcplistener net.Listener
}

func NewProxy(quicport, tcpport int) *Proxy {
	return &Proxy{
		quicport: quicport,
		tcpport:  tcpport,
	}
}

func (p *Proxy) StartProxy() {
	// 生成TLS配置
	tlsConfig := GenerateTLSConfig()
	// 设置QUIC配置
	quicConfig := &quic.Config{
		MaxIdleTimeout: 2 * time.Minute, // 空闲 2 分钟后断开
	}

	log.Printf("Proxy: 开始启动QUIC监听器于端口: %d", p.quicport)
	log.Printf("端口使用的是udp协议，做端口映射的时候请注意")
	// 启动QUIC监听器
	addr := fmt.Sprintf("0.0.0.0:%d", p.quicport)
	var err error
	p.listener, err = quic.ListenAddr(addr, tlsConfig, quicConfig)
	if err != nil {
		log.Printf("启动QUIC监听失败: %v", err)
		os.Exit(1)
	}
	// 在后台监听连接
	go p.acceptConnections()
	// 启动conn处理
	go p.handleConnection()
	// 启动心跳处理
	go p.handleHeartbeat()
	// 启动心跳检测
	go p.checkHeartbeat()
	// 启动tcp监听
	go p.dotcp()
}

func (p *Proxy) acceptConnections() {
	for {
		conn, err := p.listener.Accept(context.Background())
		if err != nil {
			log.Printf("接受连接失败: %v", err)
			continue
		}
		log.Printf("接受了新的Bridge连接: %s", conn.RemoteAddr())

		oldconn := p.conn
		p.conn = conn
		p.lastHeartbeat = time.Now()
		if oldconn != nil {
			if p.heartbeatStream != nil {
				p.heartbeatStream.Close()
			}
			oldconn.CloseWithError(quic.ApplicationErrorCode(0), "old connection")
			p.heartbeatStream = nil
		}
	}
}

func (p *Proxy) handleConnection() {
	for {
		if p.conn == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("Proxy AcceptStream")
		stream, err := p.conn.AcceptStream(context.Background())
		if err != nil || stream == nil {
			log.Printf("Proxy AcceptStream error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("Proxy AcceptStream success")
		p.heartbeatStream = stream

		log.Printf("心跳流更新: %d", stream.StreamID())
	}
}

func (p *Proxy) handleHeartbeat() {
	for {
		if p.conn == nil || p.heartbeatStream == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		// 读取1个字节
		buf := make([]byte, 1)
		_, err := p.heartbeatStream.Read(buf)
		if err != nil {
			log.Printf("读取心跳失败: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		// 如果读取到的字节是12，则认为心跳正常
		if buf[0] == 12 {
			p.lastHeartbeat = time.Now()
		}
	}
}

func (p *Proxy) checkHeartbeat() {
	for {
		if p.conn == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		if time.Since(p.lastHeartbeat) > 10*time.Second {
			log.Printf("心跳超时，关闭连接")
			p.conn.CloseWithError(quic.ApplicationErrorCode(0), "heartbeat timeout")
			p.conn = nil
			p.heartbeatStream = nil
		}
		time.Sleep(1 * time.Second)
	}
}

func (p *Proxy) dotcp() {
	addr := fmt.Sprintf("0.0.0.0:%d", p.tcpport)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("启动TCP监听失败: %v", err)
		time.Sleep(3 * time.Second)
		os.Exit(1)
	}
	p.tcplistener = listener

	for {
		conn, err := p.tcplistener.Accept()
		if err != nil {
			log.Printf("接受TCP连接失败: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		if p.conn == nil {
			log.Printf("没有bridge的quic连接，跳过")
			conn.Close()
			continue
		}
		go p.handleTCP(conn)
	}
}

var errcount = 0

func (p *Proxy) handleTCP(conn net.Conn) {
	// 创建stream 进行copy
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel() // 确保释放资源
	stream, err := p.conn.OpenStreamSync(ctx)
	if err != nil {
		log.Printf("创建stream失败: %v", err)
		conn.Close()
		errcount++
		if errcount > 3 {
			log.Printf("创建stream失败 超过3次")
			os.Exit(5)
		}
		return
	}
	errcount = 0
	// 任意一个失败 关闭连接
	done := make(chan bool)
	go func() {
		_, err := io.Copy(stream, conn)
		if err != nil {
			log.Printf("写入stream失败: %v", err)
		}
		done <- true
	}()
	go func() {
		_, err := io.Copy(conn, stream)
		if err != nil {
			log.Printf("读取stream失败: %v", err)
		}
		done <- true
	}()
	<-done
	conn.Close()
	stream.Close()
}
