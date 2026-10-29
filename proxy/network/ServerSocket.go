package network

import (
	"client/proto"
	"client/vo"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

var serverConn net.Conn // 服务端与代理端建立的控制连接，用来通知服务端主动建立工作连接
var connMap sync.Map    // key 为 sessionId，并发安全
var lock sync.Mutex     // 保护 serverConn 的读写以及对控制连接的写入

// setServerConn 线程安全地更新控制连接
func setServerConn(conn net.Conn) {
	lock.Lock()
	serverConn = conn
	lock.Unlock()
}

// writeToServerConn 线程安全地向控制连接写入，避免多 goroutine 并发写导致报文交错
func writeToServerConn(b []byte) (int, error) {
	lock.Lock()
	defer lock.Unlock()
	if serverConn == nil {
		return 0, io.ErrClosedPipe
	}
	return serverConn.Write(b)
}

// isServerConnReady 线程安全地判断控制连接是否已就绪
func isServerConnReady() bool {
	lock.Lock()
	defer lock.Unlock()
	return serverConn != nil
}

var authToken string // 代理端配置的合法 token

// SetAuthToken 设置鉴权令牌，由 main 在解析配置后调用
func SetAuthToken(token string) {
	authToken = token
}

// checkAuth 校验报文携带的 token 是否合法
func checkAuth(pkg vo.NetPackage) bool {
	if authToken == "" {
		return true // 未配置 token 时不校验
	}
	return string(pkg.Token) == authToken
}

func Opt(conn net.Conn) {
	// #5 兜底：隔离单个连接的意外 panic，避免拖垮整个代理进程。
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Opt 发生 panic 已恢复: %v", r)
		}
	}()
	var sessionId string
	piped := false // 是否已进入裸转发阶段（进入后由 rawPipe 负责关闭连接）
	defer func() {
		if !piped {
			conn.Close()
		}
		if sessionId != "" {
			connMap.Delete(sessionId)
		}
	}()

	for {
		result, err := proto.Decode2(conn)
		if err != nil {
			log.Printf("读取报文失败: %v", err)
			break
		}
		sessionId = string(result.SessionId)

		// 鉴权校验
		if !checkAuth(result) {
			log.Printf("鉴权失败, sessionId=%s", sessionId)
			rsp, _ := proto.Encode(vo.NetPackage{
				MessageType: result.MessageType,
				SessionId:   result.SessionId,
				Code:        []byte(vo.AUTH_FAILED),
				Message:     []byte(vo.AUTH_FAILED_MESSAGE),
			})
			conn.Write(rsp)
			break
		}

		switch string(result.MessageType) {
		case vo.WK_SERVER_PACAKGE: // 服务端发起心跳包
			rspData := vo.NetPackage{
				MessageType: []byte(vo.WK_SERVER_PACAKGE),
				SessionId:   result.SessionId,
				Code:        []byte(vo.SUCCESS),
				Message:     []byte(vo.SUCCESS_MESSAGE),
			}
			b, _ := proto.Encode(rspData)
			// 先登记为控制连接，再通过加锁写入，统一控制连接的并发写互斥
			setServerConn(conn)
			writeToServerConn(b)
		case vo.BUILD_SERVER_CONNECTION: // 服务端回连建立的工作连接
			log.Printf("服务端与代理端成功建立工作连接 sessionId=%s", sessionId)
			connMap.Store(sessionId, conn)
			// 该连接作为 server 工作连接存入 map，由对应 client 会话取走做裸转发。
			// 置空 sessionId，避免本 goroutine 的 defer 误删刚存入的工作连接。
			piped = true
			sessionId = ""
			return
		case vo.TRANS_DATA:
			// 方案 A：该 client 连接独占一个会话。收到握手包后配对一条 server 工作连接，
			// 之后两条连接直接裸转发，不再解析后续数据。
			serverTmpConn := createServerConnect(result)
			if serverTmpConn == nil {
				log.Printf("建立服务端工作连接失败 sessionId=%s", sessionId)
				return
			}
			piped = true
			rawPipe(conn, serverTmpConn, sessionId)
			return
		default:
			log.Printf("无效的消息类型: %s", result.MessageType)
		}
	}
}

// rawPipe 在两条连接之间做双向裸数据转发（零编解码），结束后关闭两端
func rawPipe(a net.Conn, b net.Conn, sessionId string) {
	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(b, a)
		if cw, ok := b.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		io.Copy(a, b)
		if cw, ok := a.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		}
	}()
	wg.Wait()
	log.Printf("会话结束 sessionId=%s", sessionId)
}

func createServerConnect(r vo.NetPackage) net.Conn {
	// 构造通知服务端建立工作连接的报文
	reqData := vo.NetPackage{
		SectionName: r.SectionName,
		MessageType: []byte(vo.BUILD_SERVER_CONNECTION),
		SessionId:   r.SessionId,
	}
	bytes, _ := proto.Encode(reqData)

	// 等待服务端的控制连接就绪（线程安全地读取）
	for !isServerConnReady() {
		log.Println("等待服务端与代理端建立控制连接...")
		time.Sleep(1 * time.Second)
	}

	// 通过加锁写入，避免与心跳响应等并发写控制连接导致报文交错
	if n, err := writeToServerConn(bytes); err != nil {
		log.Printf("通知服务端建立连接失败: %v", err)
		return nil
	} else {
		log.Printf("通知服务端建立连接成功 size=%d", n)
	}

	sessionKey := string(reqData.SessionId)
	// 等待服务端回连建立工作连接（带超时，避免永久阻塞）
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := connMap.LoadAndDelete(sessionKey); ok {
			return v.(net.Conn)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 超时：清理可能迟到的工作连接，避免连接与 map 条目泄漏
	if v, ok := connMap.LoadAndDelete(sessionKey); ok {
		v.(net.Conn).Close()
	}
	log.Printf("等待服务端工作连接超时 sessionId=%s", sessionKey)
	return nil
}