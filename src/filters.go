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
	// 历史版本中仅有一个 Message
	// 现改为私聊/群聊情况分别持有 message 过滤器
	PrivateMessage MessageContentFilter
	GroupMessage   MessageContentFilter
	// 为确保向后兼容，保留一个通用 Message 字段兜底
	Message MessageContentFilter
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	var useMessage *MessageContentFilter

	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		if onebotMessage.Partial.UserId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有user_id字段的private消息，过滤器被阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Private.Filter(onebotMessage.Partial.UserId) {
			useMessage = &f.PrivateMessage
			if useMessage.Mode == "" {
				useMessage = &f.Message
			}
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
			useMessage = &f.GroupMessage
			if useMessage.Mode == "" {
				useMessage = &f.Message
			}
			break //通过 id 校验
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的群消息不通过：%s\n", f.Name, onebotMessage.Partial.GroupId, onebotMessage.Partial.RawMessage)
		}
		return false
	default:
		if CONFIG.Server.Debug {
			log.Printf("不被支持的消息类型（既非群聊也非私聊……？）\n")
		}
		return true
	}

	// 若没有指定任何 message 策略或为 ON（表示放行），直接通过
	if useMessage == nil || useMessage.Mode == "" || useMessage.Mode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：直接通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	// 前缀通过检查
	if useMessage.prefixPass(onebotMessage) {
		log.Printf("%s：前缀通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		return true
	}

	// 正则匹配（支持 array 格式的多段 text）
	if onebotMessage.Partial.MessageFormat == "array" {
		for _, message := range onebotMessage.Partial.Message {
			if message.Type == MESSAGE_TYPE_TEXT {
				text := message.Data["text"].(string)
				result := f.processFilterMatchWith(useMessage, text, onebotMessage.Partial.RawMessage)
				if result != nil {
					return *result
				}
			}
		}
	} else {
		result := f.processFilterMatchWith(useMessage, onebotMessage.Partial.RawMessage, onebotMessage.Partial.RawMessage)
		if result != nil {
			return *result
		}
	}

	// 依据 message.Mode 决定默认行为（未匹配到任何正则时）
	switch useMessage.Mode {
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
	f.Private.Mode = cfg.Private.Mode
	f.Private.Ids = cfg.Private.Ids
	f.Group.Mode = cfg.Group.Mode
	f.Group.Ids = cfg.Group.Ids

	// 优先使用 private.message（如果在 private 下配置），否则回退到顶层 cfg.Message
	privateMsgCfg := cfg.Private.Message
	if privateMsgCfg.Mode == "" && (cfg.Message.Mode != "" || len(cfg.Message.Filters) > 0 || len(cfg.Message.Prefix) > 0 || cfg.Message.PrefixReplace != "") {
		privateMsgCfg = cfg.Message
	}
	f.PrivateMessage.Mode = privateMsgCfg.Mode
	f.PrivateMessage.Filters = privateMsgCfg.Filters
	f.PrivateMessage.Prefix = privateMsgCfg.Prefix
	f.PrivateMessage.PrefixReplace = privateMsgCfg.PrefixReplace

	// 同理处理 group.message
	groupMsgCfg := cfg.Group.Message
	if groupMsgCfg.Mode == "" && (cfg.Message.Mode != "" || len(cfg.Message.Filters) > 0 || len(cfg.Message.Prefix) > 0 || cfg.Message.PrefixReplace != "") {
		groupMsgCfg = cfg.Message
	}
	f.GroupMessage.Mode = groupMsgCfg.Mode
	f.GroupMessage.Filters = groupMsgCfg.Filters
	f.GroupMessage.Prefix = groupMsgCfg.Prefix
	f.GroupMessage.PrefixReplace = groupMsgCfg.PrefixReplace

	// 也设置通用 f.Message（以防外部还以 f.Message 使用）
	f.Message.Mode = cfg.Message.Mode
	f.Message.Filters = cfg.Message.Filters
	f.Message.Prefix = cfg.Message.Prefix
	f.Message.PrefixReplace = cfg.Message.PrefixReplace

	// 编译正则表达式：分别为 PrivateMessage、GroupMessage、以及通用 Message
	compile := func(mc *MessageContentFilter) {
		newFilters := []*regexp.Regexp{}
		for _, filter := range mc.Filters {
			pattern, err := regexp.Compile(filter, regexp.None)
			if err != nil {
				log.Printf("编译正则表达式：%s，出错：%v\n", filter, err)
				continue
			}
			newFilters = append(newFilters, pattern)
		}
		mc.Regexps = newFilters
	}
	compile(&f.PrivateMessage)
	compile(&f.GroupMessage)
	compile(&f.Message)

	return f
}

func (f *Filter) String() string {
	return fmt.Sprintf(`
	name: %s
	private: %s, ids: %v
	group: %s, ids: %v
	private.message: %s, filters: %v
	group.message: %s, filters: %v
	message: %s, filters: %v
	prefix(private): %s, prefix-replace(private): %s
	prefix(group): %s, prefix-replace(group): %s`,
		f.Name,
		f.Private.Mode, f.Private.Ids,
		f.Group.Mode, f.Group.Ids,
		f.PrivateMessage.Mode, f.PrivateMessage.Filters,
		f.GroupMessage.Mode, f.GroupMessage.Filters,
		f.Message.Mode, f.Message.Filters,
		f.PrivateMessage.Prefix, f.PrivateMessage.PrefixReplace,
		f.GroupMessage.Prefix, f.GroupMessage.PrefixReplace)
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
func (mc *MessageContentFilter) prefixPass(onebotMessage *OneBotMessage) bool {
	if mc == nil {
		return false
	}
	if len(mc.Prefix) == 0 {
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
func (f *Filter) processFilterMatchWith(mc *MessageContentFilter, Text, RawMessage string) *bool {
	if mc == nil {
		return nil
	}
	for _, pattern := range mc.Regexps {
		if ok, err := pattern.MatchString(Text); ok {
			switch mc.Mode {
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
