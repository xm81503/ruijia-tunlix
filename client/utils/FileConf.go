package utils

/**
  @auther xiezz
  客户端ini文件解析
*/
import (
	"client/vo"
	"log"
	"os"
	"path/filepath"

	"github.com/go-ini/ini"
)

var IniConfMap map[string]vo.IniSection

// resolveConfPath 解析配置文件路径：优先使用当前工作目录下的相对路径；
// 若不存在，则回退到「可执行文件所在目录」下同名文件，
// 以兼容在任意目录下启动（如 macOS 双击或从其他目录运行）的场景。
func resolveConfPath(filePath string) string {
	if _, err := os.Stat(filePath); err == nil {
		return filePath
	}
	exe, err := os.Executable()
	if err != nil {
		return filePath
	}
	candidate := filepath.Join(filepath.Dir(exe), filepath.Base(filePath))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return filePath
}

func ParseIniConf(filePath string) map[string]vo.IniSection {
	filePath = resolveConfPath(filePath)
	file, error := ini.Load(filePath)
	if error != nil {
		log.Fatalf("加载配置文件 %s 失败: %v", filePath, error)
	}
	sectionStrings := file.SectionStrings()
	IniConfMap = make(map[string]vo.IniSection, 8)
	for _, sectionName := range sectionStrings {
		if sectionName == "DEFAULT" {
			continue
		}
		section, error := file.GetSection(sectionName)
		mysection := new(vo.IniSection)
		if error != nil {
			log.Fatalf("读取配置段 %s 失败: %v", sectionName, error)
		}
		detailData := new(vo.SectionDetail)
		if sectionName == "common" {
			detailData.ServerAddr = section.Key("server_addr").String()
			detailData.ServerPort, _ = section.Key("server_port").Int()
			detailData.Token = section.Key("token").String()
			// TLS 默认开启；仅当显式配置 tls_enable=false 时关闭
			detailData.TlsEnable = section.Key("tls_enable").MustBool(true)
			detailData.TlsCert = section.Key("tls_cert_file").String()
			detailData.TlsKey = section.Key("tls_key_file").String()
		} else {
			detailData.LocalIp = section.Key("bind_addr").String()
			detailData.LocalPort, _ = section.Key("bind_port").Int()
			detailData.ServerName = section.Key("server_name").String()
			detailData.SK = section.Key("sk").String()
		}
		mysection.SectionName = sectionName
		mysection.DetailData = *detailData
		IniConfMap[sectionName] = *mysection
	}
	return IniConfMap
}