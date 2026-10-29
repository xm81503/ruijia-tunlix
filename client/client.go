package main

import (
	"client/network"
	"client/utils"
	"client/vo"
	"fmt"
	"log"
	"net"
	"sync"
)

var wg sync.WaitGroup

func main() {
	log.Println("welcome to my super proxy prog, let's go!")
	// 解析配置文件
	sectionMap := utils.ParseIniConf("./client.ini")

	// 为每个非 common 段绑定本地端口，接收用户连接
	for _, section := range sectionMap {
		if section.SectionName != "common" {
			wg.Add(1)
			localIp := sectionMap[section.SectionName].DetailData.LocalIp
			localPort := sectionMap[section.SectionName].DetailData.LocalPort
			address := fmt.Sprintf("%s:%d", localIp, localPort)
			go opt(address, section)
		}
	}
	wg.Wait()
}

func opt(address string, section vo.IniSection) {
	defer wg.Done()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Printf("监听 %s 失败: %v", address, err)
		return
	}
	log.Printf("开始监听 %s", address)
	for {
		connection, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接失败: %v", err)
			continue
		}
		go network.SendDataToRemoteServer(connection, section.DetailData.ServerName)
	}
}