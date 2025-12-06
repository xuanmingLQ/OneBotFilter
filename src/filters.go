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
	Name           string
	UserId         IdFilter
	GroupId        IdFilter
	PrivateMessage MessageFilter
	GroupMessage   MessageFilter
	// Message MessageFilter 直接使用各自的message配置，在check时已经自动继承
}

// 账号黑白名单过滤器
type IdFilter struct {
	IdConfig
}

// 消息内容过滤器
type MessageFilter struct {
	MessageConfig
	// MessageContentFilter // MessageTypeConfig里已经有MessageContentConfig了，直接自己带regexps好了
	Regexps []*regexp.Regexp //编译后的正则表达式
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	var usedFilter *MessageFilter

	switch onebotMessage.Partial.MessageType {
	case GROUP: // 群聊消息
		// 群黑白名单检查
		if !f.GroupId.Filter(onebotMessage.Partial.GroupId) {
			if CONFIG.Server.Debug {
				log.Printf("%s：群 %d 的消息不通过：%s\n", f.Name, onebotMessage.Partial.UserId, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		// 使用群消息过滤器
		usedFilter = &f.GroupMessage
		// 接着执行user-id黑白名单检查
		fallthrough
	case PRIVATE:
		if !f.UserId.Filter(onebotMessage.Partial.UserId) {
			if CONFIG.Server.Debug {
				log.Printf("%s：QQ %d 的消息不通过：%s\n", f.Name, onebotMessage.Partial.UserId, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		// 如果没有使用群消息过滤器，就使用私聊消息过滤器
		if usedFilter == nil {
			usedFilter = &f.PrivateMessage
		}
		break
	default:
		if CONFIG.Server.Debug {
			log.Printf("%s：message_type=%s的消息，直接放行：%s\n", f.Name, onebotMessage.Partial.MessageType, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	// 若没有指定任何 message 策略或为 ON（表示放行），直接通过
	if usedFilter == nil || usedFilter.Mode == "" || usedFilter.Mode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：直接通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}
	// 模式为off，禁止消息
	if usedFilter.Mode == OFF {
		if CONFIG.Server.Debug {
			log.Printf("%s：被禁止的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
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
	switch usedFilter.Mode {
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

// Compile 编译过滤器（从配置生成过滤器）
// 保持函数签名不变，但会把 private/group 的 message 分别设置
func (f *Filter) Compile(cfg BotAppsConfig) *Filter {
	f.Name = cfg.Name
	f.UserId = IdFilter{cfg.UserId}
	f.GroupId = IdFilter{cfg.GroupId}
	f.PrivateMessage.Compile(cfg.PrivateMessage)
	f.GroupMessage.Compile(cfg.GroupMessage)
	return f
}

func (f *MessageFilter) Compile(cfg MessageConfig) *MessageFilter {
	f.MessageConfig = cfg
	newFilters := []*regexp.Regexp{}
	for _, filter := range f.Filters {
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
user-id: %s , ids: %v
group-id: %s , ids: %v
private-message: %s
	filters: [ %s ]
	prefix: [ %s ], replace: %s
group-message: %s
	filters: [ %s ]
	prefix: [ %s ], replace: %s`,
		f.Name,
		f.UserId.Mode, f.UserId.Ids,
		f.GroupId.Mode, f.GroupId.Ids,
		f.PrivateMessage.Mode,
		strings.Join(f.PrivateMessage.Filters, ", "),
		strings.Join(f.PrivateMessage.Prefix, ", "), f.PrivateMessage.PrefixReplace,
		f.GroupMessage.Mode,
		strings.Join(f.GroupMessage.Filters, ", "),
		strings.Join(f.GroupMessage.Prefix, ", "), f.GroupMessage.PrefixReplace,
	)
}

// 黑白名单过滤
func (idf *IdFilter) Filter(id int64) bool {
	if id == 0 { // id有问题，直接通过
		return true
	}
	switch idf.Mode {
	case "", ON:
		return true
	case OFF:
		return false
	case WHITELIST:
		return slices.Contains(idf.Ids, id)
	case BLACKLIST:
		return !slices.Contains(idf.Ids, id)
	}
	return true //配置有问题，直接通过吧
}

// 前缀通过功能，直接由MessageTypeFilter来处理
func (mf *MessageFilter) prefixPass(onebotMessage *OneBotMessage) bool {
	if mf == nil {
		return false
	}
	if len(mf.Prefix) == 0 {
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
		for _, prefix := range mf.Prefix {
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
	text := mf.PrefixReplace + textOld[len(prefix):]
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
func (mf *MessageFilter) processFilter(Name, Text, RawMessage string) *bool {
	if mf == nil {
		return nil
	}
	for _, pattern := range mf.Regexps {
		if ok, err := pattern.MatchString(Text); ok {
			switch mf.Mode {
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
