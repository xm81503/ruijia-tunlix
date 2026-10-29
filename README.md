# ruijia-tunlix

一个用 Go 实现的轻量级内网穿透 / 反向代理工具（简化版 frp）。通过部署在公网的**代理端**，把内网服务暴露给外部用户访问，支持 **TLS 加密**、**Token 鉴权**和**断线自愈**。

代码简单，很容易读懂，关键是可以满足90%的应用场景

## 架构

```
          用户流量                    公网 (TLS)                  内网
用户 ──────────────▶ proxy 代理端 ◀══════════════▶ server 服务端 ──────▶ 内网服务
                      (公网监听)                    (内网出口)          (如 Web/DB)

client 客户端 ◀══════════════▶ proxy 代理端
 (暴露本地服务)        公网 (TLS)
```

三个独立模块（各自为独立 Go module）：

| 模块         | 角色  | 部署位置  | 说明                       |
| ---------- | --- | ----- | ------------------------ |
| **proxy**  | 代理端 | 公网服务器 | 对外监听，路由用户流量；作为 TLS 服务端   |
| **server** | 服务端 | 内网    | 连接 proxy，把流量转发到内网目标服务    |
| **client** | 客户端 | 本地    | 监听本地端口，把本地服务流量经 proxy 转出 |

> 说明：`client` 与 `server` 是两种使用方式，分别对应「暴露本地端口」和「暴露内网目标服务」两类场景，均通过 `proxy` 中转。

## 数据面设计

- **握手 + 裸转发**：控制面用「4 字节大端长度头 + JSON」自定义协议完成握手、心跳、鉴权；数据面握手后切换为 `io.Copy` 裸转发（零编解码），每个用户会话独占一条到代理的连接。
- **TLS**：默认开启，只加密穿越公网的链路（client↔proxy、server↔proxy）；本地回环段（client↔本地用户、server↔内网服务）保持纯 TCP。proxy 未配置证书时自动生成内存自签证书，客户端默认 `InsecureSkipVerify`，开箱即用。
- **鉴权**：各端携带 `common` 段 `token`，proxy 校验，不匹配则拒绝。
- **断线自愈**：server 控制连接断开后自动重连并重启心跳。
- **稳定性**：各连接处理 goroutine 带 `recover` 兜底，单连接故障不影响整体进程。

## 环境要求

- Go 1.20+
- 构建命令演示基于 `make`（可选，也可直接用 `go build`）

## 安装部署

## 快速开始

### 1. 编译

本机编译（按当前系统架构，产物输出到 `bin/`）：

```bash
# 编译全部模块
make build

# 或单独编译
make proxy
make server
make client
```

不使用 make 时：

```bash
cd proxy  && go build -o ../bin/proxy  .
cd server && go build -o ../bin/server .
cd client && go build -o ../bin/client .
```

> Go 编译的是特定「系统 + 架构」的原生二进制，**不能跨平台直接拷贝运行**（如 Linux 产物无法在 macOS 运行）。要为其他平台出包，请使用下面的交叉编译。

### 1.1 交叉编译（跨平台出包）

仓库的 `bin/` 目录下已附带**编译好的各平台二进制**，开箱即用，可按上表直接选用对应平台的文件，无需自行编译。

如需自行编译，一条命令为全部目标平台编译全部模块，产物命名为 `<模块>-<os>-<arch>[.exe]`，统一放在 `bin/`：

```bash
make cross
```

Windows / macOS / Linux 宿主均可运行（Makefile 已按宿主系统自动选择环境变量设置语法）。目标平台：

| 平台              | 产物示例                       | 适用                      |
| --------------- | -------------------------- | ----------------------- |
| `darwin/arm64`  | `client-darwin-arm64`      | Apple Silicon Mac（M 系列） |
| `darwin/amd64`  | `client-darwin-amd64`      | Intel Mac               |
| `linux/amd64`   | `client-linux-amd64`       | x86_64 Linux            |
| `linux/arm64`   | `client-linux-arm64`       | ARM64 Linux             |
| `windows/amd64` | `client-windows-amd64.exe` | 64 位 Windows            |

> 拷贝到目标机后，需将对应二进制与同名配置文件（`proxy.ini` / `server.ini` / `client.ini`）放在**同一目录**。程序会优先按当前工作目录查找配置，找不到时自动回退到「可执行文件所在目录」，因此从任意目录启动或双击运行均可。

### 2. 配置

三个模块各有一个同名 `.ini` 配置文件。

**proxy.ini**（代理端，需要准备一台公网机器，内网端和客户端都能访问这台公网机器）

```ini
[common]
server_addr = 0.0.0.0        ; 对外监听地址
server_port = 8111           ; 对外监听端口
token = 56@1uixkj16789kl     ; 鉴权令牌，三端需一致
tls_enable = true            ; TLS 开关，默认开启
#tls_cert_file = server.crt  ; 可选：正式证书，留空则自动生成内存自签证书
#tls_key_file  = server.key
```

**server.ini**（内网服务端）

```ini
[common]
server_addr = 公网代理IP
server_port = 8111
token = 56@1uixkj16789kl

[test]                       ; section 名由 client 的 server_name 指定
bind_addr = 127.0.0.1        ; 内网目标服务地址
bind_port = 7890             ; 内网目标服务端口
sk = test
```

**client.ini**（本地客户端）

```ini
[common]
server_addr = 公网代理IP
server_port = 8111
token = 56@1uixkj16789kl
con_num = 1

[SC_BAOBIAO]
bind_addr = 127.0.0.1        ; 本地监听地址
bind_port = 9000             ; 本地监听端口
server_name = test           ; 对应 server.ini 中的 section 名，用于路由
sk = test
```

> 路由关系：client 的 `server_name` → 匹配 server 对应 `[section]` → 转发到该段的 `bind_addr:bind_port`。

### 3. 启动

按顺序启动（代理端需先就绪）：

```bash
make run-proxy     # 公网：启动代理端
make run-server    # 内网：启动服务端
make run-client    # 本地：启动客户端
```

或直接运行编译产物（二进制与对应 `.ini` 放同一目录即可，程序会自动在可执行文件所在目录查找配置）：

```bash
cd proxy  && ../bin/proxy
cd server && ../bin/server
cd client && ../bin/client
```

## 安全说明

默认 TLS 模式只保证**加密传输**（防窃听），客户端 `InsecureSkipVerify` 不校验代理端身份（不防中间人），与 frp 默认行为一致。若需防中间人，可在 proxy 配置固定证书并在客户端启用证书校验。

## 免责声明

1. 本项目仅供学习、研究及合法用途的内网穿透与端口转发使用，**严禁用于任何违法违规活动**，包括但不限于：未授权访问他人计算机系统、网络攻击、窃取他人数据、传播违法信息等。
2. 使用者应确保在**拥有合法授权**的网络环境中部署和运行本项目，并对自己的所有行为及由此产生的一切后果**承担全部法律责任**。
3. 本项目按"原样"提供，不作出任何明示或暗示的保证。对于使用者滥用本项目所造成的任何直接或间接损失、纠纷或法律责任，**作者不承担任何责任**。
4. 如使用者违反所在国家或地区的法律法规，责任由使用者自行承担。若因此引发法律纠纷，与本项目作者无关。
5. **下载或使用本项目即视为已阅读并同意本声明的全部内容。** 若不同意，请立即停止使用并删除本项目。

## 捐助

如果这个项目对你有帮助，欢迎请作者喝杯咖啡，你的支持是持续维护的动力 ❤️，

如果你有特殊的要求或者相关的技术问题可以添加 TG   @sanyough


<img src="assets/2026-10-29-15-26-52-image.png" title="" alt="" width="119">             

微信支付二维码