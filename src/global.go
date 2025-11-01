package onebotfilter

import (
	"log"

	"github.com/spf13/viper"
)

// 过滤器模式
const (
	DEFAULT   = "default"
	ON        = "on"
	OFF       = "off"
	WHITELIST = "whitelist"
	BLACKLIST = "blacklist"
)

// 消息类型
const (
	PRIVATE = "pivate"
	GROUP   = "group"
)

// 消息格式
// 消息的内容类型
const (
	MESSAGE_FORMAT_ARRAY = "array"
	MESSAGE_TYPE_TEXT    = "text"
)

// 布尔值
var (
	TRUE  = true
	FALSE = false
)

// 配置文件相关
var (
	VP          *viper.Viper
	CONFIG      Config
	ALL_FILTERS []*Filter
)

type WsMsg struct {
	MsgType int
	MsgData []byte
}

func AddFilter(filter *Filter) {
	for _, f := range ALL_FILTERS {
		if f.Name == filter.Name {
			return
		}
	}
	ALL_FILTERS = append(ALL_FILTERS, filter)
}
func RemoveFilter(name string) {
	for i, f := range ALL_FILTERS {
		if f.Name == name {
			ALL_FILTERS = append(ALL_FILTERS[:i], ALL_FILTERS[i+1:]...)
			return
		}
	}
}

// 重新加载所有过滤器
func ReLoadFilters() error {
	for _, botApp := range CONFIG.BotApps {
		//检查配置
		err := botApp.Check()
		if err != nil {
			continue
		}
		for _, filter := range ALL_FILTERS {
			if filter.Name == botApp.Name {
				filter.Compile(botApp)
				log.Printf("已重新加载过滤器：%s\n", filter.String())
				break
			}
		}
	}
	log.Printf("重新加载过滤器，共有%d个\n", len(ALL_FILTERS))
	return nil
}
