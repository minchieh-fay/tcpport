package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/quic-go/quic-go"
)

type Bridge struct {
	serverAddr     string
	targetAddr     string
	conn           quic.Connection
	hearbeatStream quic.Stream
}

func NewBridge(serverAddr, targetAddr string) *Bridge {
	return &Bridge{
		serverAddr: serverAddr,
		targetAddr: targetAddr,
	}
}

func (b *Bridge) StartBridge() {
	// TLS配置
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-tunnel"},
	}
	// QUIC配置
	quicConfig := &quic.Config{}
	// quic 连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // 确保释放资源
	quicConn, err := quic.DialAddr(ctx, b.serverAddr, tlsConfig, quicConfig)
	if err != nil {
		log.Printf("quic 连接失败 代理地址无法连接 %s: %v", b.serverAddr, err)
		time.Sleep(3 * time.Second)
		os.Exit(1)
	}

	// 创建心跳流
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1() // 确保释放资源
	heartbeatStream, err := quicConn.OpenStreamSync(ctx1)
	if err != nil {
		log.Printf("创建心跳流失败: %v", err)
		quicConn.CloseWithError(quic.ApplicationErrorCode(0), "create heartbeat stream failed")
		time.Sleep(3 * time.Second)
		os.Exit(1)
	}
	b.conn = quicConn
	b.hearbeatStream = heartbeatStream
	// 启动心跳检测
	go b.sendHeartbeat()
	// 处理quicConn的stream
	go b.handleStream()
}

func (b *Bridge) sendHeartbeat() {
	errCount := 0
	for {
		time.Sleep(1 * time.Second)
		if b.conn == nil || b.hearbeatStream == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		// 发送心跳
		_, err := b.hearbeatStream.Write([]byte{12})
		if err != nil {
			log.Printf("发送心跳失败: %v", err)
			time.Sleep(1 * time.Second)
			errCount++
			if errCount > 5 {
				log.Printf("心跳失败次数过多，关闭连接, 程序即将退出")
				b.conn.CloseWithError(quic.ApplicationErrorCode(0), "heartbeat failed")
				os.Exit(1)
			}
			continue
		}
		errCount = 0
	}
}

func (b *Bridge) handleStream() {
	for {
		if b.conn == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		stream, err := b.conn.AcceptStream(context.Background())
		if err != nil || stream == nil {
			log.Printf("接受stream失败: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		// 处理stream
		go b.handleStreamCopy(stream)
	}
}

func (b *Bridge) handleStreamCopy(stream quic.Stream) {
	// 打开 targetAddr
	targetConn, err := net.Dial("tcp", b.targetAddr)
	if err != nil {
		log.Printf("连接目标地址失败: %v", err)
		stream.Close()
		return
	}
	// io copy// 任意失败就关闭2边
	done := make(chan bool)
	go func() {
		_, err := io.Copy(targetConn, stream)
		if err != nil {
			log.Printf("写入目标地址失败: %v", err)
		}
		done <- true
	}()
	go func() {
		_, err := io.Copy(stream, targetConn)
		if err != nil {
			log.Printf("写入目标地址失败: %v", err)
		}
		done <- true
	}()
	<-done
	stream.Close()
	targetConn.Close()
}
