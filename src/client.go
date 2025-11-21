package onebotfilter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type WsClient struct {
	Name      string
	conn      *websocket.Conn
	filter    *Filter
	readChan  chan WsMsg //从bot应用端读取到的消息
	writeChan chan WsMsg //写入到bot应用端的消息
}

// 连接到反向ws服务端，转发消息，并使用过滤器
func WsClientHandler(wss *WsServer, cfg BotAppsConfig) {
	//检查配置
	err := cfg.Check()
	if err != nil {
		log.Printf("%s的配置有问题: %v\n", cfg.Name, err)
		return
	}
	//onebot header
	header := http.Header{}
	header.Set("x-self-id", CONFIG.Server.BotId)
	header.Set("authorization", fmt.Sprintf("Bearer %s", cfg.AccessToken))
	header.Set("user-agent", CONFIG.Server.UserAgent)
	header.Set("x-client-role", "Universal")
	//filter
	filter := (&Filter{Name: cfg.Name}).Compile(cfg)
	AddFilter(filter)
	defer RemoveFilter(filter.Name)
	//client
	for { //循环重连，转发消息
		log.Printf("正在连接：%s\n", cfg.Name)

		dialer := &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			ReadBufferSize:   CONFIG.Server.BufferSize,
			WriteBufferSize:  CONFIG.Server.BufferSize,
		}
		conn, _, err := dialer.Dial(cfg.Uri, header)
		if err != nil {
			log.Printf("连接%s异常: %v\n", cfg.Name, err)
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
			continue
		}
		client := &WsClient{
			Name:      cfg.Name,
			conn:      conn,
			filter:    filter,
			readChan:  make(chan WsMsg),
			writeChan: make(chan WsMsg),
		}
		err = wss.AddWsClient(client) //添加到客户端列表
		if err != nil {
			log.Printf("连接%s异常: %v\n", cfg.Name, err)
			client.conn.Close()
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
			continue
		}
		ctx, ctxCancel := context.WithCancel(context.Background())
		go client.readLoop(ctx, wss)
		go client.writeLoop(ctx)
		log.Printf("已连接到：%s，加载的过滤器：%s\n", cfg.Name, filter.String())
		for {
			mt, msg, err := client.conn.ReadMessage()
			if err != nil {
				log.Printf("从%s读取消息出错：%v\n", cfg.Name, err)
				client.conn.Close()             //关闭客户端
				wss.RemoveWsClient(client.Name) //从客户端列表中删除
				time.Sleep(5 * time.Second)
				break
			}
			client.readChan <- WsMsg{mt, msg}
		}
		client.close(ctxCancel)
	}
}
func (wc *WsClient) WriteMessage(mt int, msg []byte) error {
	if wc.conn == nil {
		return errors.New("没有连接到bot应用端")
	}
	wc.writeChan <- WsMsg{mt, msg}
	return nil
}
func (wc *WsClient) close(ctxCancel context.CancelFunc) {
	ctxCancel()
	if wc.conn != nil {
		wc.conn.Close()
	}
	close(wc.readChan)
	close(wc.writeChan)
	wc.conn = nil
}

// 处理从bot应用端读取的消息
func (wc *WsClient) readLoop(ctx context.Context, wss *WsServer) {
	for {
		select {
		case msg := <-wc.readChan:
			//转发给OneBot客户端
			if err := wss.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
				log.Println("写入到OneBot客户端出错：", err)
			}
		case <-ctx.Done():
			return

		}
	}
}

// 处理发送给bot应用端的消息
func (wc *WsClient) writeLoop(ctx context.Context) {
	for {
		select {
		case msg := <-wc.writeChan:
			if msg.MsgType == websocket.TextMessage {

				// 解析onebot的消息
				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if onebotMessage == nil {
					//解析出错的消息也直接放行
					if err := wc.conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
						log.Printf("向%s发送消息出错：%v\n", wc.Name, err)
					}
					continue
				}
				// 通常的消息
				if onebotMessage.Partial.RawMessage != "" {
					if wc.filter.Filter(onebotMessage) {
						//过滤器通过，发送
						if err := wc.conn.WriteJSON(onebotMessage.Intact); err != nil {
							log.Printf("向%s发送消息出错：%v\n", wc.Name, err)
						}
					}
					continue
				}
			}
			//其他消息直接放行
			if err := wc.conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
				log.Printf("向%s发送消息出错：%v\n", wc.Name, err)
			}
		case <-ctx.Done():
			return
		}
	}
}
