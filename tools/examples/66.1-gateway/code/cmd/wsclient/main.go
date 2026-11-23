// wsclient 是最小 WebSocket 客户端，用来验证网关的下行投递与 redirect。
//
//	go run ./cmd/wsclient -url 'ws://127.0.0.1:18081/ws?ticket=demo-1' -hold 5s
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := flag.String("url", "", "websocket 地址，含 ticket 查询参数")
	hold := flag.Duration("hold", 5*time.Second, "连接保持时长")
	flag.Parse()
	ws, resp, err := websocket.DefaultDialer.Dial(*url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		fmt.Printf("DIAL_FAIL status=%d err=%v\n", status, err)
		os.Exit(1)
	}
	defer ws.Close()
	fmt.Println("CONNECTED", *url)
	_ = ws.SetReadDeadline(time.Now().Add(*hold))
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			fmt.Println("CLOSED", err)
			return
		}
		fmt.Println("FRAME", string(data))
	}
}
