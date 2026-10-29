package main

import (
	clientnet "client/network"
	"client/utils"
	"fmt"
	"log"
	"proxy/network"
)

func main() {
	log.Println("proxy start!")
	// 绑定本地端口
	sectionMap := utils.ParseIniConf("./proxy.ini")
	localIp := sectionMap["common"].DetailData.ServerAddr
	localPort := sectionMap["common"].DetailData.ServerPort
	network.SetAuthToken(sectionMap["common"].DetailData.Token)
	address := fmt.Sprintf("%s:%d", localIp, localPort)
	log.Printf("proxy listening on %s", address)

	common := sectionMap["common"].DetailData
	listen, err := clientnet.TLSListen(address, common.TlsCert, common.TlsKey, common.TlsEnable)
	if err != nil {
		log.Fatalf("监听 %s 失败: %v", address, err)
	}

	for {
		connection, err := listen.Accept()
		if err != nil {
			log.Printf("接受连接失败: %v", err)
			continue
		}
		go network.Opt(connection)
	}
}