package main

import (
	"client/network"
	"client/proto"
	"client/utils"
	"client/vo"
	"io"
	"log"
	"net"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
)

var lock sync.Mutex
var authToken string

func SendServerHeartPackage(conn net.Conn, stop <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("SendServerHeartPackage 发生 panic 已恢复: %v", r)
		}
	}()
	for {
		select {
		case <-stop:
			return
		default:
		}

		sessionId := uuid.NewV4().String()
		message := vo.NetPackage{
			MessageType: []byte(vo.WK_SERVER_PACAKGE),
			SessionId:   []byte(sessionId),
			Token:       []byte(authToken),
			Data:        []byte("hello"),
		}
		reqByte, err := proto.Encode(message)
		if err != nil {
			log.Printf("编码心跳包失败: %v", err)
			time.Sleep(time.Second * 20)
			continue
		}

		lock.Lock()
		_, err = conn.Write(reqByte)
		lock.Unlock()
		if err != nil {
			log.Printf("心跳包发送失败: %v", err)
			return
		}
		log.Println("心跳包发送成功")

		select {
		case <-stop:
			return
		case <-time.After(time.Second * 20):
		}
	}
}

func main() {
	log.Println("start server!")
	sectionMap := utils.ParseIniConf("./server.ini")
	remoteIp := sectionMap["common"].DetailData.ServerAddr
	remotePort := sectionMap["common"].DetailData.ServerPort
	authToken = sectionMap["common"].DetailData.Token

	// 断线自愈：控制连接断开后自动重连并重启心跳
	for {
		runControlLoop(sectionMap, remoteIp, remotePort)
		log.Println("与代理端的控制连接已断开，5 秒后重连...")
		time.Sleep(time.Second * 5)
	}
}

// runControlLoop 建立控制连接、启动心跳并处理报文，连接断开时返回
func runControlLoop(sectionMap map[string]vo.IniSection, remoteIp string, remotePort int) {
	connection := network.CreateConnect(remoteIp, remotePort)
	defer connection.Close()

	// 心跳包处理，连接断开时心跳协程退出
	stopHeart := make(chan struct{})
	go SendServerHeartPackage(connection, stopHeart)
	defer close(stopHeart)

	for {
		reqData, err := proto.Decode2(connection)
		if err != nil {
			log.Printf("读取报文失败: %v", err)
			return
		}
		switch string(reqData.MessageType) {
		case vo.BUILD_SERVER_CONNECTION:
			log.Println("接收: BUILD_SERVER_CONNECTION")
			proto.PrintNetPackage(reqData)

			// #1 校验：SectionName 来自代理端下发的报文，属不可信输入。
			// 若本地配置中不存在该 section，拒绝该请求，避免用空配置去拨号。
			sectionName := string(reqData.SectionName)
			section, ok := sectionMap[sectionName]
			if !ok {
				log.Printf("未知的 section: %s，忽略该建连请求", sectionName)
				continue
			}
			localIp := section.DetailData.LocalIp
			localPort := section.DetailData.LocalPort
			localConnection := network.CreateLocalConnect(localIp, localPort)

			serverConn := network.CreateConnect(remoteIp, remotePort)

			// 回连携带 token 用于代理端鉴权
			buildRsp := vo.NetPackage{
				MessageType: []byte(vo.BUILD_SERVER_CONNECTION),
				SectionName: reqData.SectionName,
				SessionId:   reqData.SessionId,
				Token:       []byte(authToken),
			}
			bytes, err := proto.Encode(buildRsp)
			if err != nil {
				log.Printf("编码建连报文失败: %v", err)
				serverConn.Close()
				localConnection.Close()
				continue
			}
			if _, err := serverConn.Write(bytes); err != nil {
				log.Printf("发送建连报文失败: %v", err)
				serverConn.Close()
				localConnection.Close()
				continue
			}

			go proxyForwardData(serverConn, localConnection, string(reqData.SessionId))
		case vo.WK_SERVER_PACAKGE:
			log.Println("接收心跳响应: WK_SERVER_PACAKGE")
			proto.PrintNetPackage(reqData)
		default:
			log.Printf("未知的消息类型: %s", reqData.MessageType)
		}
	}
}

func proxyForwardData(proxyConn net.Conn, localConn net.Conn, sessionId string) {
	// #5 兜底：隔离单个会话的意外 panic，避免拖垮整个进程。
	defer func() {
		if r := recover(); r != nil {
			log.Printf("proxyForwardData 发生 panic 已恢复 sessionId=%s: %v", sessionId, r)
		}
	}()
	// 方案 A：建连握手已完成，之后代理连接与本地服务连接之间直接裸转发（零编解码）。
	defer func() {
		proxyConn.Close()
		localConn.Close()
	}()

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		io.Copy(localConn, proxyConn)
		if cw, ok := localConn.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()
	go func() {
		defer waitGroup.Done()
		io.Copy(proxyConn, localConn)
		if cw, ok := proxyConn.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()
	waitGroup.Wait()
	log.Printf("会话结束 sessionId=%s", sessionId)
}