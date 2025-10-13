package onebotfilter

import (
	"fmt"
	"log"
	"slices"
	"strings"

	regexp "github.com/dlclark/regexp2"
)

type MessageTypeFilter struct {
	Mode string //on、 off、 whitelist or blacklist
	Ids  []int64
}

type MessageFilter struct {
	Mode          string           // whitelist or blacklist
	Filters       []string         //正则表达式原始字符串
	Regexps       []*regexp.Regexp //编译后的正则表达式
	Prefix        []string
	PrefixReplace string
}

type Filter struct {
	Name    string
	Private MessageTypeFilter
	Group   MessageTypeFilter
	Message MessageFilter
}

func (f *Filter) Filter(rawMessage string, onebotMessage map[string]interface{}) bool {
	messageType, ok := onebotMessage["message_type"].(string)
	if !ok {
		if CONFIG.Server.Debug {
			log.Printf("%s：没有message_type字段的消息，直接放行：%s\n", f.Name, rawMessage)
		}
		return true
	}
	switch messageType {
	case "private":
		userId, ok := onebotMessage["user_id"].(float64)
		if !ok {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有user_id字段的private消息，直接阻止：%s\n", f.Name, rawMessage)
			}
			return false
		}
		if f.Private.Filter(int64(userId)) {
			break //通过
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的私聊不通过：%s\n", f.Name, int64(userId), rawMessage)
		}
		return false
	case "group":
		groupId, ok := onebotMessage["group_id"].(float64)
		if !ok {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有group_id字段的group消息，直接阻止：%s\n", f.Name, rawMessage)
			}
			return false
		}
		if f.Group.Filter(int64(groupId)) {
			break //通过
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的群消息不通过：%s\n", f.Name, int64(groupId), rawMessage)
		}
		return false
	default:
		if CONFIG.Server.Debug {
			log.Printf("%s：message_type=%s的消息，直接放行：%s\n", f.Name, messageType, rawMessage)
		}
		return true
	}
	if f.Message.Mode == "" {
		log.Printf("%s：直接通过的消息：%s\n", f.Name, rawMessage)
	}
	if f.Message.prefixPass(onebotMessage) {
		log.Printf("%s：前缀通过的消息：%s\n", f.Name, rawMessage)
		return true
	}
	for _, pattern := range f.Message.Regexps {
		if ok, err := pattern.MatchString(rawMessage); ok {
			if f.Message.Mode == WHITELIST {
				log.Printf("%s：白名单的消息：%s\n", f.Name, rawMessage)
				return true
			} else if f.Message.Mode == BLACKLIST {
				log.Printf("%s：黑名单的消息：%s\n", f.Name, rawMessage)
				return false
			}
		} else if err != nil {
			log.Printf("%s的过滤器%s正则匹配出错的消息：%s\n", f.Name, pattern.String(), rawMessage)
		}
	}
	if f.Message.Mode == WHITELIST {
		if CONFIG.Server.Debug {
			log.Printf("%s：不在白名单中的消息：%s\n", f.Name, rawMessage)
		}
		return false
	} else if f.Message.Mode == BLACKLIST {
		log.Printf("%s：不在黑名单中的消息：%s\n", f.Name, rawMessage)
		return true
	}
	log.Printf("%s的message.mode配置异常，必须为whitelist或blacklist\n", f.Name)
	return false
}

// 编译过滤器
func (f *Filter) Compile(cfg BotAppsConfig) *Filter {
	f.Private.Mode = cfg.Private.Mode
	f.Private.Ids = cfg.Private.Ids
	f.Group.Mode = cfg.Group.Mode
	f.Group.Ids = cfg.Group.Ids
	f.Message.Mode = cfg.Message.Mode
	f.Message.Filters = cfg.Message.Filters
	f.Message.Prefix = cfg.Message.Prefix
	f.Message.PrefixReplace = cfg.Message.PrefixReplace
	//编译正则表达式
	newFilters := []*regexp.Regexp{}
	for _, filter := range f.Message.Filters {
		pattern, err := regexp.Compile(filter, regexp.None)
		if err != nil {
			log.Printf("编译正则表达式：%s，出错：%v\n", filter, err)
			continue
		}
		newFilters = append(newFilters, pattern)
	}
	f.Message.Regexps = newFilters
	return f
}
func (f *Filter) String() string {
	return fmt.Sprintf(`name: %s
private: %s, ids: %v
group: %s, ids: %v
message: %s, filters: %v
prefix: %s, prefix-replace: %s`, f.Name,
		f.Private.Mode, f.Private.Ids,
		f.Group.Mode, f.Group.Ids,
		f.Message.Mode, f.Message.Filters,
		f.Message.Prefix, f.Message.PrefixReplace)
}

// detail_type过滤
func (f *MessageTypeFilter) Filter(id int64) bool {
	switch f.Mode {
	case "", ON:
		return true
	case OFF:
		return false
	case WHITELIST:
		return slices.Contains(f.Ids, id)
	case BLACKLIST:
		return !slices.Contains(f.Ids, id)
	}
	return true
}

// 前缀通过功能
func (f *MessageFilter) prefixPass(onebotMessage map[string]interface{}) bool {
	if len(f.Prefix) == 0 {
		return false
	}
	rawMessage, ok := onebotMessage["raw_message"].(string)
	if !ok {
		return false
	}
	//查找匹配的前缀
	prefix := (func() string {
		for _, prefix := range f.Prefix {
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(rawMessage, prefix) {
				return prefix
			}
		}
		return ""
	})()
	//没有匹配的前缀
	if prefix == "" {
		return false
	}
	if onebotMessage["message_format"].(string) != "array" {
		return false
	}
	message := onebotMessage["message"].([]interface{})
	if len(message) <= 0 {
		return false
	}
	msg0, ok := message[0].(map[string]interface{})
	if !ok {
		return false
	}
	if msg0["type"].(string) != "text" {
		return false
	}
	msg0data, ok := msg0["data"].(map[string]interface{})
	if !ok {
		return false
	}
	text := msg0data["text"].(string)
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	text = f.PrefixReplace + text[len(prefix):]
	msg0data["text"] = text
	if strings.TrimSpace(text) == "" {
		onebotMessage["message"] = message[1:]
	}
	onebotMessage["raw_message"] = f.PrefixReplace + rawMessage[len(prefix):]
	return true
}
