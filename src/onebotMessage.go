package onebotfilter

import (
	"encoding/json"
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
	return oneBotMessage
}

type OneBotMessagePartial struct {
	MessageType   string           `json:"message_type"`
	MessageFormat string           `json:"message_format"`
	Message       []MessageContent `json:"message"`
	UserId        int64            `json:"user_id"`
	GroupId       int64            `json:"group_id"`
	RawMessage    string           `json:"raw_message"`
}
type MessageContent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}
