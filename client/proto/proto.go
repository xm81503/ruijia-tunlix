package proto

import (
	"bytes"
	"client/vo"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net"
)

func PrintNetPackage(message vo.NetPackage) {
	log.Printf("MessageType:%s SectionName:%s SessionId:%s Code:%s Message:%s",
		message.MessageType, message.SectionName, message.SessionId,
		message.Code, message.Message)
}

// Encode 将消息编码为 [4 字节大端长度头 + JSON body]
func Encode(message vo.NetPackage) ([]byte, error) {
	b, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	var pkg = new(bytes.Buffer)
	if err = binary.Write(pkg, binary.BigEndian, int32(len(b))); err != nil {
		return nil, err
	}
	if err = binary.Write(pkg, binary.BigEndian, b); err != nil {
		return nil, err
	}
	return pkg.Bytes(), nil
}

// Decode2 从连接中读取一个完整报文
func Decode2(conn net.Conn) (vo.NetPackage, error) {
	headLen := make([]byte, 4)
	// 使用 ReadFull 确保读满 4 字节长度头，避免 TCP 短读
	if _, err := io.ReadFull(conn, headLen); err != nil {
		return vo.NetPackage{}, err
	}
	length := binary.BigEndian.Uint32(headLen)

	body := make([]byte, length)
	// 使用 ReadFull 确保读满整个报文体，避免大包被拆分导致 JSON 解析失败
	if _, err := io.ReadFull(conn, body); err != nil {
		return vo.NetPackage{}, err
	}

	out := new(vo.NetPackage)
	if err := json.Unmarshal(body, out); err != nil {
		return vo.NetPackage{}, err
	}
	return *out, nil
}