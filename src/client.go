package onebotfilter

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WsClienter interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(int, []byte) error
	GetName() string
	Close() error
}

type WsClient struct {
	Name   string
	Conn   *websocket.Conn
	filter *Filter
	Prefix string
	Debug  bool
	mutex  sync.Mutex
}

func (wc *WsClient) ReadMessage() (int, []byte, error) {
	return wc.Conn.ReadMessage()
}
func (wc *WsClient) GetName() string {
	return wc.Name
}
func (wc *WsClient) Close() error {
	if wc.Conn != nil {
		return wc.Conn.Close()
	}
	return nil
}

const (
	WhitelistFilter = iota
	BlacklistFilter
)

type WihtelistClient struct{ WsClient }
type BlacklistClient struct{ WsClient }

// 连接到反向ws服务端，转发消息，并使用过滤器
func ConnectWsClient(wss *WsServer, filterType int, clientConfig ClientConfig, serverConfig ServerConfig) {
	//onebot header
	header := http.Header{}
	header.Set("x-self-id", string(serverConfig.BotId))
	header.Set("authorization", fmt.Sprintf("Bearer %s", clientConfig.AccessToken))
	header.Set("user-agent", serverConfig.UserAgent)
	header.Set("x-client-role", "Universal")
	//filter
	filter := (&Filter{Name: clientConfig.Name}).Load(clientConfig.Filters)
	AddFilter(filter)
	defer RemoveFilter(filter.Name)
	//client
	var wsClient WsClienter
	for { //循环重连，转发消息
		log.Printf("正在连接：%s\n", clientConfig.Name)
		conn, _, err := websocket.DefaultDialer.Dial(clientConfig.Uri, header)
		if err != nil {
			log.Printf("连接%s异常: %v\n", clientConfig.Name, err)
			time.Sleep(5 * time.Second)
			continue
		}
		client := WsClient{
			Name:   clientConfig.Name,
			Conn:   conn,
			filter: filter,
			Prefix: clientConfig.Prefix,
			Debug:  serverConfig.Debug,
		}
		var filterTypeName string
		switch filterType {
		case WhitelistFilter:
			wsClient = &WihtelistClient{
				client,
			}
			filterTypeName = "白名单"
		case BlacklistFilter:
			wsClient = &BlacklistClient{
				client,
			}
			filterTypeName = "黑名单"
		default:
			log.Printf("未知的过滤器类型")
			conn.Close()
			return
		}
		err = wss.AddWsClient(wsClient) //添加到客户端列表
		if err != nil {
			log.Printf("连接%s异常: %v\n", clientConfig.Name, err)
			wsClient.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		log.Printf("已连接到：%s，加载的%s：%s，前缀：%s\n", clientConfig.Name, filterTypeName, filter.String(), clientConfig.Prefix)
		for {
			mt, msg, err := wsClient.ReadMessage()
			if err != nil {
				log.Printf("从%s读取消息出错：%v\n", clientConfig.Name, err)
				wsClient.Close()                      //关闭客户端
				wss.RemoveWsClient(clientConfig.Name) //从客户端列表中删除
				time.Sleep(5 * time.Second)
				break
			}
			if err := wss.WriteMessage(mt, msg); err != nil {
				log.Println("写入到OneBot客户端出错：", err)
			}
		}
	}
}

func (wlc *WihtelistClient) WriteMessage(mt int, msg []byte) error {
	wlc.mutex.Lock()
	defer wlc.mutex.Unlock()
	//解析json格式的消息
	onebotMessage := make(map[string]interface{})
	err := json.Unmarshal(msg, &onebotMessage)
	if err != nil {
		return err
	}
	//通常的消息
	if rawMessage, ok := onebotMessage["raw_message"].(string); ok {
		if prefixPass(wlc.Prefix, onebotMessage) {
			log.Printf("%s前缀通过的消息：%s\n", wlc.Name, rawMessage)
			return wlc.Conn.WriteJSON(onebotMessage)
		}
		//放行白名单匹配成功的消息
		if wlc.filter.Filter(rawMessage) {
			log.Printf("%s白名单的消息：%s\n", wlc.Name, rawMessage)
			return wlc.Conn.WriteMessage(mt, msg)
		}
		//阻止不匹配的消息
		if wlc.Debug {
			log.Printf("%s阻止的消息：%s\n", wlc.Name, rawMessage)
		}
		return nil
	}
	//其他消息直接放行
	return wlc.Conn.WriteMessage(mt, msg)
}
func (blc *BlacklistClient) WriteMessage(mt int, msg []byte) error {
	blc.mutex.Lock()
	defer blc.mutex.Unlock()
	//解析json格式的消息
	onebotMessage := make(map[string]interface{})
	err := json.Unmarshal(msg, &onebotMessage)
	if err != nil {
		return err
	}
	//通常的消息
	if rawMessage, ok := onebotMessage["raw_message"].(string); ok {
		if prefixPass(blc.Prefix, onebotMessage) {
			log.Printf("%s前缀通过的消息：%s\n", blc.Name, rawMessage)
			return blc.Conn.WriteJSON(onebotMessage)
		}
		//阻止黑名单匹配成功的消息
		if blc.filter.Filter(rawMessage) {
			log.Printf("%s黑名单的消息：%s\n", blc.Name, rawMessage)
			return nil
		}
		//放行不匹配的消息
		if blc.Debug {
			log.Printf("%s放行的消息：%s\n", blc.Name, rawMessage)
		}
	}
	//其他消息直接放行
	return blc.Conn.WriteMessage(mt, msg)
}
