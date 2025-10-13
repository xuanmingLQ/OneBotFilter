package onebotfilter

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

type WsServer struct {
	Conn      *websocket.Conn
	WsClients []*WsClient
	readChan  chan WsMsg //从OneBot客户端读取到的消息
	writeChan chan WsMsg //写入到OneBot客户端的消息
	// mutex     sync.Mutex
}

// 处理与OneBot客户端的连接
func (wss *WsServer) WsServerHandler() error {
	ctx, ctxCancel := context.WithCancel(context.Background())
	wss.readChan = make(chan WsMsg)
	wss.writeChan = make(chan WsMsg)
	go wss.readLoop(ctx)       //开启读取OneBot客户端消息协程
	go wss.writeLoop(ctx)      //开启写入OneBot客户端消息携程
	defer wss.close(ctxCancel) //注册关闭方法
	for {
		mt, msg, err := wss.Conn.ReadMessage()
		if err != nil {
			return err
		}
		wss.readChan <- WsMsg{mt, msg}
	}
	// return errors.New("读取消息循环已结束")
}

// 向OneBot客户端写入消息
func (wss *WsServer) WriteMessage(mt int, msg []byte) error {
	if wss.Conn == nil {
		return errors.New("没有连接到OneBot客户端")
	}
	wss.writeChan <- WsMsg{mt, msg}
	return nil
}

// 添加bot应用端
func (wss *WsServer) AddWsClient(wsClient *WsClient) error {
	// wss.mutex.Lock()
	// defer wss.mutex.Unlock()
	for _, c := range wss.WsClients {
		if c.Name == wsClient.Name {
			return fmt.Errorf("已经连接过%s", wsClient.Name)
		}
	}
	wss.WsClients = append(wss.WsClients, wsClient)
	return nil
}

// 删除bot应用端
func (wss *WsServer) RemoveWsClient(name string) {
	// wss.mutex.Lock()
	// defer wss.mutex.Unlock()
	for i, c := range wss.WsClients {
		if c.Name == name {
			wss.WsClients = append(wss.WsClients[:i], wss.WsClients[i+1:]...) //从列表中删除
			return
		}
	}
}

// 关闭连接
func (wss *WsServer) close(ctxCancel context.CancelFunc) {
	ctxCancel()
	if wss.Conn != nil {
		wss.Conn.Close()
	}
	wss.Conn = nil
}

// 处理从OneBot客户端读取到的消息
func (wss *WsServer) readLoop(ctx context.Context) {
	for {
		select {
		case msg := <-wss.readChan:
			// 转发给所有bot应用
			for _, wsClient := range wss.WsClients {
				go func(wsClient *WsClient, mt int, msg []byte) {
					if err := wsClient.WriteMessage(mt, msg); err != nil {
						log.Printf("向 %s 发送消息出错：%v\n", wsClient.Name, err)
					}
				}(wsClient, msg.MsgType, msg.MsgData)
			}
		case <-ctx.Done():
			return
		}
	}
}

// 处理写入OneBot客户端的消息
func (wss *WsServer) writeLoop(ctx context.Context) {
	for {
		select {
		case msg := <-wss.writeChan:
			if err := wss.Conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
				log.Println("写入到OneBot客户端出错：", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
