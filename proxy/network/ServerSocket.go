package network

import (
	"client/proto"
	"client/vo"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// 多 server 支持：
// 每台 server 可负责一组 section，启动后用一条控制连接登记它负责的所有 section 名。
// 代理端据此维护 section 名 -> 控制连接 的路由表，按用户请求携带的 SectionName
// 把建连通知发往正确的 server 控制连接。
var connMap sync.Map // key 为 sessionId 的工作连接（全局 uuid，不同 server 间不会碰撞），并发安全

var (
	ctrlLock     sync.Mutex              // 保护 sectionConns 读写，以及对控制连接的并发写入
	sectionConns = map[string]net.Conn{} // section 名 -> 负责它的 server 控制连接
)

// registerControlConn 将一条控制连接登记为 sections 中各 section 的负责连接。
// 同名 section 的后到连接覆盖先到者（视为该 server 重连）。
func registerControlConn(conn net.Conn, sections []string) {
	ctrlLock.Lock()
	defer ctrlLock.Unlock()
	for _, name := range sections {
		if name == "" {
			continue
		}
		sectionConns[name] = conn
	}
}

// unregisterControlConn 控制连接断开时，移除其登记的全部 section（仅移除仍指向该 conn 的条目，
// 避免误删已被同名 server 重连覆盖的新连接）。
func unregisterControlConn(conn net.Conn) {
	ctrlLock.Lock()
	defer ctrlLock.Unlock()
	for name, c := range sectionConns {
		if c == conn {
			delete(sectionConns, name)
		}
	}
}

// writeToSectionConn 线程安全地向负责指定 section 的控制连接写入。
// 找不到对应控制连接时返回错误（该 section 当前无在线 server）。
func writeToSectionConn(section string, b []byte) (int, error) {
	ctrlLock.Lock()
	defer ctrlLock.Unlock()
	conn, ok := sectionConns[section]
	if !ok || conn == nil {
		return 0, io.ErrClosedPipe
	}
	return conn.Write(b)
}

// writeToConn 线程安全地向指定控制连接写入，与 writeToSectionConn 共用同一把锁，
// 避免多 goroutine 并发写同一条控制连接导致报文交错。
func writeToConn(conn net.Conn, b []byte) (int, error) {
	ctrlLock.Lock()
	defer ctrlLock.Unlock()
	return conn.Write(b)
}

// isSectionReady 线程安全地判断指定 section 是否已有在线的 server 控制连接。
func isSectionReady(section string) bool {
	ctrlLock.Lock()
	defer ctrlLock.Unlock()
	conn, ok := sectionConns[section]
	return ok && conn != nil
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
		// 若本连接是某些 section 的控制连接，断开时解除其路由登记，
		// 避免 isSectionReady 对已死控制连接误判为就绪。
		unregisterControlConn(conn)
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
			// 心跳包的 Data 携带本 server 负责的 section 列表（逗号分隔）。
			// 先按列表登记 section -> 控制连接 的路由，再回复心跳。
			sections := strings.Split(string(result.Data), ",")
			registerControlConn(conn, sections)
			// 心跳回复直接回给本控制连接。注册表的锁保护的是跨 goroutine 的控制连接写，
			// 这里的写与 createServerConnect 的写互斥由 writeToConn 统一加锁。
			if _, err := writeToConn(conn, b); err != nil {
				log.Printf("回复心跳失败: %v", err)
			}
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

	section := string(r.SectionName)
	// 等待负责该 section 的 server 控制连接就绪（带超时避免永久阻塞）
	ctrlDeadline := time.Now().Add(30 * time.Second)
	for !isSectionReady(section) {
		if time.Now().After(ctrlDeadline) {
			log.Printf("等待 section=%s 的服务端控制连接就绪超时 sessionId=%s", section, string(reqData.SessionId))
			return nil
		}
		log.Printf("等待负责 section=%s 的服务端控制连接...", section)
		time.Sleep(1 * time.Second)
	}

	// 向负责该 section 的控制连接发建连通知；加锁写入避免与心跳等并发写导致报文交错
	if n, err := writeToSectionConn(section, bytes); err != nil {
		log.Printf("通知服务端建立连接失败 section=%s: %v", section, err)
		return nil
	} else {
		log.Printf("通知服务端建立连接成功 section=%s size=%d", section, n)
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
