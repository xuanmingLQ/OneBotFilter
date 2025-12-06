package main

import (
	"fmt"
	"log"
	"net/http"
	filter "onebotfiler/src"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wss = &filter.WsServer{}
)

func handleLocal(w http.ResponseWriter, r *http.Request) {
	if wss.Conn != nil {
		http.Error(w, "只能连接一个OneBot客户端", http.StatusForbidden)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
		return
	}
	// defer conn.Close()
	wss.Conn = conn
	log.Println("已连接到OneBot客户端")
	err = wss.WsServerHandler()
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
	}
	//循环结束
	log.Println("OneBot客户端连接已断开")
}

func main() {
	err := filter.LoadConfigVP("config.yaml")
	if err != nil {
		log.Fatal("加载配置异常:", err)
	}
	upgrader.ReadBufferSize = filter.CONFIG.Server.BufferSize
	upgrader.WriteBufferSize = filter.CONFIG.Server.BufferSize
	http.HandleFunc(filter.CONFIG.Server.Suffix, handleLocal)
	go func() {
		for _, bacfg := range filter.CONFIG.BotApps {
			go filter.WsClientHandler(wss, bacfg)
		}
	}()

	log.Printf("OneBotFilter已启动 ws://%s:%d%s\n", filter.CONFIG.Server.Host, filter.CONFIG.Server.Port, filter.CONFIG.Server.Suffix)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", filter.CONFIG.Server.Host, filter.CONFIG.Server.Port), nil); err != nil {
		log.Fatal("监听服务出错:", err)
	}
}
