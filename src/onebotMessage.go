package onebotfilter

import (
	"encoding/json"
	"log"
)

type OneBotMessage struct {
	Raw     []byte
	Partial OneBotMessagePartial
	Intact  map[string]json.RawMessage
}

func ParseOneBotMessage(Raw []byte) *OneBotMessage {
	oneBotMessage := &OneBotMessage{
		Raw: Raw,
	}
	if err := json.Unmarshal(Raw, &oneBotMessage.Intact); err != nil {
		return nil
	}
	if err := json.Unmarshal(Raw, &oneBotMessage.Partial); err != nil {
		return nil
	}
	switch oneBotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageArray); err != nil {
			log.Printf("将%s解析为array失败\n", oneBotMessage.Partial.UnDecodedMessage)
			return nil
		}
	case MESSAGE_FORMAT_STRING:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageString); err != nil {
			log.Printf("将%s解析为string失败\n", oneBotMessage.Partial.UnDecodedMessage)
			return nil
		}
	default: //未知的format或没有format
		return nil
	}
	return oneBotMessage
}

type OneBotMessagePartial struct {
	MessageType      string           `json:"message_type"`
	MessageFormat    string           `json:"message_format"`
	UnDecodedMessage json.RawMessage  `json:"message"`
	MessageArray     []MessageContent `json:"-"`
	MessageString    string           `json:"-"`
	UserId           int64            `json:"user_id"`
	GroupId          int64            `json:"group_id"`
	RawMessage       string           `json:"raw_message"`
}
type MessageContent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}
