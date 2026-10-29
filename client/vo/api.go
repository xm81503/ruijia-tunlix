package vo

const GET_REMOTE_ADDR = "1000"         //获取远程地址
const REGISTER_SERVER_ADDR = "1001"    //注册地址
const TRANS_DATA = "1003"              //转发传输数据
const WK_SERVER_PACAKGE = "1004"       //SERVER端与代理端建立的
const BUILD_SERVER_CONNECTION = "1005" //客服务通知代理，代理通知服务端与代理建立连接
const TRANS_DATA_RSP = "1007"

const SUCCESS = "0000"
const SUCCESS_MESSAGE = "ok"

const AUTH_FAILED = "1001"           //鉴权失败
const AUTH_FAILED_MESSAGE = "unauthorized"

/*
登陆  响应
*/
type SectionDetail struct {
	ConnNum    int    //建立与代理服务器的连接数
	LocalIp    string //本地IP
	LocalPort  int    //本地端口
	ServerAddr string //远程IP
	ServerPort int    //远程端口
	ServerName string //使用远程服务的sectionName
	Token      string //common 段的鉴权令牌
	SK         string //业务段访问密钥
	TlsEnable  bool   //是否启用 TLS（common 段，默认 true）
	TlsCert    string //TLS 证书文件路径（proxy 端可选，留空则自动生成自签证书）
	TlsKey     string //TLS 私钥文件路径（proxy 端可选）
}

type IniSection struct {
	SectionName string
	DetailData  SectionDetail
}

/*
报文请求数据
*/
type NetPackage struct {
	MessageType []byte //消息类型
	SectionName []byte //
	SessionId   []byte //连接会话ID
	Code        []byte //0000表成功
	Message     []byte //描述
	Data        []byte //请求数据
	Token       []byte //鉴权令牌
}

/*
*
报文响应数据
*/