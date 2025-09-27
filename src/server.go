package onebotfilter

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type WsServer struct {
	Conn      *websocket.Conn
	mutex     sync.Mutex
	WsClients []WsClienter
}

func (wss *WsServer) ReadMessageLoop() error {
	for {
		mt, msg, err := wss.Conn.ReadMessage()
		if err != nil {
			log.Printf("读取OneBot客户端消息异常：\n", err)
			break
		}
		// 转发给所有远程
		for _, wsClient := range wss.WsClients {
			go func(wsClient WsClienter, mt int, msg []byte) {
				if err := wsClient.WriteMessage(mt, msg); err != nil {
					log.Printf("向 %s 发送消息出错：%v\n", wsClient.GetName(), err)
				}
			}(wsClient, mt, msg)
		}
	}
	return errors.New("读取消息循环已结束")
}
func (wss *WsServer) WriteMessage(mt int, msg []byte) error {
	wss.mutex.Lock()
	defer wss.mutex.Unlock()
	if wss.Conn == nil {
		return errors.New("没有连接到OneBot客户端")
	}
	return wss.Conn.WriteMessage(mt, msg)
}
func (wss *WsServer) AddWsClient(wsClient WsClienter) error {
	wss.mutex.Lock()
	defer wss.mutex.Unlock()
	for _, c := range wss.WsClients {
		if c.GetName() == wsClient.GetName() {
			return fmt.Errorf("已经连接过%s", wsClient.GetName())
		}
	}
	wss.WsClients = append(wss.WsClients, wsClient)
	return nil
}
func (wss *WsServer) RemoveWsClient(name string) {
	wss.mutex.Lock()
	defer wss.mutex.Unlock()
	for i, c := range wss.WsClients {
		if c.GetName() == name {
			wss.WsClients = append(wss.WsClients[:i], wss.WsClients[i+1:]...) //从列表中删除
			return
		}
	}
}
func (wss *WsServer) Close() {
	wss.mutex.Lock()
	if wss.Conn != nil {
		wss.Conn.Close()
	}
	wss.Conn = nil
	wss.mutex.Unlock()
}
