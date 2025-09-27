package onebotfilter

import "strings"

func prefixPass(prefix string, onebotMessage map[string]interface{}) bool {
	if prefix == "" {
		return false
	}
	rawMessage, ok := onebotMessage["raw_message"].(string)
	if !ok {
		return false
	}
	if !strings.HasPrefix(rawMessage, prefix) {
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
	text = text[len(prefix):]
	msg0data["text"] = text
	if strings.TrimSpace(text) == "" {
		onebotMessage["message"] = message[1:]
	}
	onebotMessage["raw_message"] = rawMessage[len(prefix):]
	return true
}
