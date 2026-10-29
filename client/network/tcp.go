package network

import (
	"client/proto"
	"client/utils"
	"client/vo"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
)

func CreateConnect(remoteIp string, remotePort int) net.Conn {
	tlsEnable := utils.IniConfMap["common"].DetailData.TlsEnable
reconnect:
	address := fmt.Sprintf("%s:%d", remoteIp, remotePort)
	proxyCon, err := TLSDial(address, tlsEnable)
	if err != nil {
		log.Printf("连接 %s 失败: %v, 5 秒后重连", address, err)
		time.Sleep(time.Second * 5)
		goto reconnect
	}
	log.Printf("连接服务器 [%s] 成功", address)
	return proxyCon
}

// CreateLocalConnect 建立到本地内网服务的普通 TCP 连接（不走 TLS）。
func CreateLocalConnect(ip string, port int) net.Conn {
reconnect:
	address := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", address, time.Second*90)
	if err != nil {
		log.Printf("连接本地服务 %s 失败: %v, 5 秒后重连", address, err)
		time.Sleep(time.Second * 5)
		goto reconnect
	}
	log.Printf("连接本地服务 [%s] 成功", address)
	return conn
}

/*
转发数据到内网
*/
func SendDataToRemoteServer(localCon net.Conn, sectionName string) {
	// #5 兜底：隔离单个用户会话的意外 panic，避免拖垮整个客户端进程。
	defer func() {
		if r := recover(); r != nil {
			log.Printf("SendDataToRemoteServer 发生 panic 已恢复 section=%s: %v", sectionName, r)
		}
	}()
	remoteIp := utils.IniConfMap["common"].DetailData.ServerAddr
	remotePort := utils.IniConfMap["common"].DetailData.ServerPort
	remoteConn := CreateConnect(remoteIp, remotePort)
	proxyForwardData(localCon, remoteConn, sectionName)
}

func proxyForwardData(localConn net.Conn, remoteConn net.Conn, sectionName string) {
	// 转发结束后关闭两端连接，避免 fd 泄漏
	defer localConn.Close()
	defer remoteConn.Close()

	token := utils.IniConfMap["common"].DetailData.Token
	sessionId := uuid.NewV4().String()

	// 方案 A：每个用户会话独占一条到代理的连接。
	// 先发一个握手包（JSON，Data 为空）协商路由，之后数据面纯裸转发，不再逐包编码。
	handshake := vo.NetPackage{
		MessageType: []byte(vo.TRANS_DATA),
		SectionName: []byte(sectionName),
		SessionId:   []byte(sessionId),
		Token:       []byte(token),
	}
	hsByte, err := proto.Encode(handshake)
	if err != nil {
		log.Printf("编码握手包失败: %v", err)
		return
	}
	if _, err := remoteConn.Write(hsByte); err != nil {
		log.Printf("发送握手包失败: %v", err)
		return
	}

	// 握手完成后，本地连接与代理连接之间直接裸转发
	rawPipe(localConn, remoteConn, sessionId)
}

// rawPipe 在两条连接之间做双向裸数据转发（零编解码）
func rawPipe(a net.Conn, b net.Conn, sessionId string) {
	var gWait sync.WaitGroup
	gWait.Add(2)

	go func() {
		defer gWait.Done()
		if _, err := io.Copy(b, a); err != nil && err != io.EOF {
			log.Printf("裸转发 a->b 结束 sessionId=%s: %v", sessionId, err)
		}
		// 一端读到 EOF 后，关闭另一端写方向，促使对向 goroutine 退出
		if cw, ok := b.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()

	go func() {
		defer gWait.Done()
		if _, err := io.Copy(a, b); err != nil && err != io.EOF {
			log.Printf("裸转发 b->a 结束 sessionId=%s: %v", sessionId, err)
		}
		if cw, ok := a.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()

	gWait.Wait()
}