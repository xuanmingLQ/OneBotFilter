package onebotfilter

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"

	regexp "github.com/dlclark/regexp2"
)

type Filter struct {
	Name    string
	Private MessageTypeFilter
	Group   MessageTypeFilter
	// Message MessageContentFilter 直接使用各自的message配置，在check时已经自动继承
}

type MessageTypeFilter struct {
	MessageTypeConfig
	// MessageContentFilter // MessageTypeConfig里已经有MessageContentConfig了，直接自己带regexps好了
	Regexps []*regexp.Regexp //编译后的正则表达式
}

// Deprecated 已弃用的通用message过滤器
type MessageContentFilter struct {
	MessageContentConfig
	Regexps []*regexp.Regexp //编译后的正则表达式
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	var usedFilter *MessageTypeFilter

	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		if onebotMessage.Partial.UserId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有user_id字段的private消息，过滤器被阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Private.Filter(onebotMessage.Partial.UserId) {
			usedFilter = &f.Private
			break
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
			// id 通过，继续 message 过滤
			usedFilter = &f.Group
			break //通过 id 校验
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

	// 若没有指定任何 message 策略或为 ON（表示放行），直接通过
	if usedFilter == nil || usedFilter.Message.Mode == "" || usedFilter.Message.Mode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：直接通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	// 前缀通过检查
	if usedFilter.prefixPass(onebotMessage) {
		log.Printf("%s：前缀通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		return true
	}
	// 正则匹配（支持 array 格式的多段 text）
	if onebotMessage.Partial.MessageFormat == "array" {
		for _, message := range onebotMessage.Partial.MessageArray {
			if message.Type == MESSAGE_TYPE_TEXT {
				text := strings.TrimSpace(message.Data["text"].(string))
				result := usedFilter.processFilter(f.Name, text, onebotMessage.Partial.RawMessage)
				if result != nil {
					return *result
				}
			}
		}
	} else {
		result := usedFilter.processFilter(f.Name, strings.TrimSpace(onebotMessage.Partial.MessageString), onebotMessage.Partial.RawMessage)
		if result != nil {
			return *result
		}
	}

	// 依据 message.Mode 决定默认行为（未匹配到任何正则时）
	switch usedFilter.Message.Mode {
	case WHITELIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在白名单中的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	case BLACKLIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在黑名单中的消息（默认允许）：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}
	log.Printf("%s的message.mode配置异常，必须为whitelist或blacklist\n", f.Name)
	return false
}

// Compile 编译过滤器（从配置生成过滤器）
// 保持函数签名不变，但会把 private/group 的 message 分别设置
func (f *Filter) Compile(cfg BotAppsConfig) *Filter {
	f.Name = cfg.Name
	f.Private.Compile(cfg.Private)
	f.Group.Compile(cfg.Group)
	return f
}

func (f *MessageTypeFilter) Compile(cfg MessageTypeConfig) *MessageTypeFilter {
	f.MessageTypeConfig = cfg
	newFilters := []*regexp.Regexp{}
	for _, filter := range f.Message.Filters {
		pattern, err := regexp.Compile(filter, regexp.None)
		if err != nil {
			log.Printf("编译正则表达式：%s，出错：%v\n", filter, err)
			continue
		}
		newFilters = append(newFilters, pattern)
	}
	f.Regexps = newFilters
	return f
}
func (f *Filter) String() string {
	return fmt.Sprintf(`
	name: %s
	private: %s , ids: %v
		message: %s , filters: %s
		prefix: %s , replace: %s
	group: %s , ids: %v
		message: %s , filters: %s
		prefix: %v , replace: %s`,
		f.Name,
		f.Private.Mode, f.Private.Ids,
		f.Private.Message.Mode, strings.Join(f.Private.Message.Filters, ", "),
		strings.Join(f.Private.Message.Prefix, ", "), f.Private.Message.PrefixReplace,
		f.Group.Mode, f.Group.Ids,
		f.Group.Message.Mode, strings.Join(f.Group.Message.Filters, ", "),
		strings.Join(f.Group.Message.Prefix, ", "), f.Group.Message.PrefixReplace)
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

// 前缀通过功能，直接由MessageTypeFilter来处理
func (f *MessageTypeFilter) prefixPass(onebotMessage *OneBotMessage) bool {
	if f == nil {
		return false
	}
	if len(f.Message.Prefix) == 0 {
		return false
	}
	var textOld string         // 查找到前缀的消息段
	var index int              // 查找到前缀的消息段索引
	var message MessageContent // 消息段内容
	var err error
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		for index, message = range onebotMessage.Partial.MessageArray {
			if message.Type != MESSAGE_TYPE_TEXT {
				continue
			}
			textOld = strings.TrimSpace(message.Data["text"].(string))
			break
		}
	case MESSAGE_FORMAT_STRING:
		textOld = strings.TrimSpace(onebotMessage.Partial.MessageString)
	default:
		return false
	}
	if textOld == "" { // 没有text类型的消息段
		return false
	}
	// 查找匹配的前缀
	prefix := (func() string {
		for _, prefix := range f.Message.Prefix {
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
	text := f.Message.PrefixReplace + textOld[len(prefix):]
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		onebotMessage.Partial.MessageArray[index].Data["text"] = text
		if strings.TrimSpace(text) == "" {
			onebotMessage.Partial.MessageArray = append(onebotMessage.Partial.MessageArray[:index], onebotMessage.Partial.MessageArray[index+1:]...)
		}
		// 修改后的消息重新打包成json
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageArray)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	case MESSAGE_FORMAT_STRING:
		onebotMessage.Partial.MessageString = text
		// 修改后的消息重新打包成json
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageString)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	}
	// 把原始消息也修改掉，这里可能会出现修改错误，就不管了
	onebotMessage.Partial.RawMessage = strings.Replace(onebotMessage.Partial.RawMessage, textOld, text, 1)
	onebotMessage.Intact["raw_message"], err = json.Marshal(onebotMessage.Partial.RawMessage)
	if err != nil {
		log.Println("将修改后的消息转为json字符串出错", err)
		return false
	}
	return true
}

// 处理对单个text消息的正则匹配
func (f *MessageTypeFilter) processFilter(Name, Text, RawMessage string) *bool {
	if f == nil {
		return nil
	}
	for _, pattern := range f.Regexps {
		if ok, err := pattern.MatchString(Text); ok {
			switch f.Message.Mode {
			case WHITELIST:
				log.Printf("%s：白名单的消息：%s\n", Name, RawMessage)
				return &TRUE
			case BLACKLIST:
				log.Printf("%s：黑名单的消息：%s\n", Name, RawMessage)
				return &FALSE
			}
		} else if err != nil {
			log.Printf("过滤器%s正则匹配出错的消息：%s\n", pattern.String(), RawMessage)
		}
	}
	return nil
}

// 前缀通过功能
// Deprecated 已弃用的通用message过滤器
func (mc *MessageContentFilter) prefixPass(onebotMessage *OneBotMessage) bool {
	if mc == nil {
		return false
	}
	if len(mc.Prefix) == 0 {
		return false
	}
	var textOld string         // 查找到前缀的消息段
	var index int              // 查找到前缀的消息段索引
	var message MessageContent // 消息段内容
	var err error
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		for index, message = range onebotMessage.Partial.MessageArray {
			if message.Type != MESSAGE_TYPE_TEXT {
				continue
			}
			textOld = strings.TrimSpace(message.Data["text"].(string))
			break
		}
	case MESSAGE_FORMAT_STRING:
		textOld = strings.TrimSpace(onebotMessage.Partial.MessageString)
	default:
		return false
	}
	if textOld == "" { // 没有text类型的消息段
		return false
	}
	// 查找匹配的前缀
	prefix := (func() string {
		for _, prefix := range mc.Prefix {
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
	text := mc.PrefixReplace + textOld[len(prefix):]
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		onebotMessage.Partial.MessageArray[index].Data["text"] = text
		if strings.TrimSpace(text) == "" {
			onebotMessage.Partial.MessageArray = append(onebotMessage.Partial.MessageArray[:index], onebotMessage.Partial.MessageArray[index+1:]...)
		}
		// 修改后的消息重新打包成json
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageArray)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	case MESSAGE_FORMAT_STRING:
		onebotMessage.Partial.MessageString = text
		// 修改后的消息重新打包成json
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageString)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	}
	// 把原始消息也修改掉，这里可能会出现修改错误，就不管了
	onebotMessage.Partial.RawMessage = strings.Replace(onebotMessage.Partial.RawMessage, textOld, text, 1)
	onebotMessage.Intact["raw_message"], err = json.Marshal(onebotMessage.Partial.RawMessage)
	if err != nil {
		log.Println("将修改后的消息转为json字符串出错", err)
		return false
	}
	return true
}
