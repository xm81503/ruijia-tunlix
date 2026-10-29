package proto

import (
	"bytes"
	"client/vo"
	"net"
	"testing"
)

// roundTrip 通过 net.Pipe 把编码后的报文写入一端，从另一端解码读出
func roundTrip(t *testing.T, msg vo.NetPackage) vo.NetPackage {
	t.Helper()
	encoded, err := Encode(msg)
	if err != nil {
		t.Fatalf("Encode 失败: %v", err)
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		client.Write(encoded)
	}()

	got, err := Decode2(server)
	if err != nil {
		t.Fatalf("Decode2 失败: %v", err)
	}
	return got
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	msg := vo.NetPackage{
		MessageType: []byte(vo.TRANS_DATA),
		SectionName: []byte("test"),
		SessionId:   []byte("session-123"),
		Code:        []byte(vo.SUCCESS),
		Message:     []byte(vo.SUCCESS_MESSAGE),
		Token:       []byte("secret-token"),
		Data:        []byte("hello world"),
	}
	got := roundTrip(t, msg)

	if string(got.MessageType) != string(msg.MessageType) {
		t.Errorf("MessageType 不一致: got %s want %s", got.MessageType, msg.MessageType)
	}
	if string(got.SessionId) != string(msg.SessionId) {
		t.Errorf("SessionId 不一致: got %s want %s", got.SessionId, msg.SessionId)
	}
	if string(got.Token) != string(msg.Token) {
		t.Errorf("Token 不一致: got %s want %s", got.Token, msg.Token)
	}
	if !bytes.Equal(got.Data, msg.Data) {
		t.Errorf("Data 不一致: got %s want %s", got.Data, msg.Data)
	}
}

// TestDecodeLargePayload 验证超过单次读取缓冲的大包能被完整读出，
// 覆盖 io.ReadFull 修复的 TCP 短读问题。
func TestDecodeLargePayload(t *testing.T) {
	large := bytes.Repeat([]byte("A"), 64*1024) // 64KB，远超常见 MTU/缓冲
	msg := vo.NetPackage{
		MessageType: []byte(vo.TRANS_DATA),
		SessionId:   []byte("big"),
		Data:        large,
	}
	got := roundTrip(t, msg)

	if !bytes.Equal(got.Data, large) {
		t.Errorf("大包数据不一致: got %d bytes want %d bytes", len(got.Data), len(large))
	}
}