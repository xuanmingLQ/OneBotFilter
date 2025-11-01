package onebotfilter

import (
	"encoding/json"
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

type MessageContentFilter struct {
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
	Message MessageContentFilter
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		if onebotMessage.Partial.UserId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有user_id字段的private消息，直接阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Private.Filter(onebotMessage.Partial.UserId) {
			break //通过
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的私聊不通过：%s\n", f.Name, onebotMessage.Partial.UserId, onebotMessage.Partial.RawMessage)
		}
		return false
	case GROUP:
		if onebotMessage.Partial.GroupId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有group_id字段的group消息，直接阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Group.Filter(onebotMessage.Partial.GroupId) {
			break //通过
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的群消息不通过：%s\n", f.Name, onebotMessage.Partial.GroupId, onebotMessage.Partial.RawMessage)
		}
		return false
	default:
		if CONFIG.Server.Debug {
			log.Printf("%s：message_type=%s的消息，直接放行：%s\n", f.Name, onebotMessage.Partial.MessageType, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	if f.Message.Mode == "" || f.Message.Mode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：直接通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
	}
	if f.Message.prefixPass(onebotMessage) {
		log.Printf("%s：前缀通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		return true
	}
	if onebotMessage.Partial.MessageFormat == "array" {
		for _, message := range onebotMessage.Partial.Message {
			if message.Type == "text" {
				text := message.Data["text"].(string)
				result := f.processFilterMatch(text, onebotMessage.Partial.RawMessage)
				if result != nil {
					return *result
				}
			}
		}
	} else {
		result := f.processFilterMatch(onebotMessage.Partial.RawMessage, onebotMessage.Partial.RawMessage)
		if result != nil {
			return *result
		}
	}
	switch f.Message.Mode {
	case WHITELIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在白名单中的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	case BLACKLIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在黑名单中的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
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
	return fmt.Sprintf(`
	name: %s
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
func (f *MessageContentFilter) prefixPass(onebotMessage *OneBotMessage) bool {
	if len(f.Prefix) == 0 {
		return false
	}
	if onebotMessage.Partial.MessageFormat != MESSAGE_FORMAT_ARRAY {
		//只支持这种消息
		return false
	}
	var textOld string         // 查找到前缀的消息段
	var index int              // 查找到前缀的消息段索引
	var message MessageContent // 消息段内容
	for index, message = range onebotMessage.Partial.Message {
		if message.Type != MESSAGE_TYPE_TEXT {
			continue
		}
		textOld = message.Data["text"].(string)
		break
	}
	if textOld == "" { // 没有text类型的消息段
		return false
	}
	// 查找匹配的前缀
	prefix := (func() string {
		for _, prefix := range f.Prefix {
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(textOld, prefix) {
				return prefix
			}
		}
		return ""
	})()
	//没有匹配的前缀
	if prefix == "" {
		return false
	}
	// 修改匹配到前缀的消息段
	text := f.PrefixReplace + textOld[len(prefix):]
	onebotMessage.Partial.Message[index].Data["text"] = text
	if strings.TrimSpace(text) == "" {
		onebotMessage.Partial.Message = append(onebotMessage.Partial.Message[:index], onebotMessage.Partial.Message[index+1:]...)
	}
	// 把原始消息也修改掉，这里可能会出现修改错误，就不管了
	onebotMessage.Partial.RawMessage = strings.Replace(onebotMessage.Partial.RawMessage, textOld, text, 1)
	var err error
	// 修改后的消息重新打包成json
	onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.Message)
	if err != nil {
		log.Println("将修改后的消息转为json字符串出错", err)
		return false
	}
	onebotMessage.Intact["raw_message"], err = json.Marshal(onebotMessage.Partial.RawMessage)
	if err != nil {
		log.Println("将修改后的消息转为json字符串出错", err)
		return false
	}
	return true
}

// 处理对单个text消息的正则匹配
func (f *Filter) processFilterMatch(Text, RawMessage string) *bool {
	for _, pattern := range f.Message.Regexps {
		if ok, err := pattern.MatchString(Text); ok {
			switch f.Message.Mode {
			case WHITELIST:
				log.Printf("%s：白名单的消息：%s\n", f.Name, RawMessage)
				return &TRUE
			case BLACKLIST:
				log.Printf("%s：黑名单的消息：%s\n", f.Name, RawMessage)
				return &FALSE
			}
		} else if err != nil {
			log.Printf("%s的过滤器%s正则匹配出错的消息：%s\n", f.Name, pattern.String(), RawMessage)
		}
	}
	return nil
}
