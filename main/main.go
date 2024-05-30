package main

import (
	douyinlive "DouyinLive"
	"DouyinLive/generated/douyin"
	"DouyinLive/utils"
	"encoding/hex"
	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"sync"
)

var agentlist = make(map[string]*websocket.Conn)
var mu sync.Mutex // 使用互斥锁来保护用户列表
var unknown bool

func main() {
	var port string

	pflag.StringVar(&port, "port", "18080", "ws端口")
	var room string
	pflag.StringVar(&room, "room", "****", "房间号")
	var unknown bool
	pflag.BoolVar(&unknown, "unknown", false, "未知源pb消息是否输出")
	pflag.Parse()
	log.Println(unknown)
	// 创建WebSocket升级器
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有CORS请求，实际应用中应根据需要设置
		},
	}

	// 设置WebSocket路由
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(upgrader, w, r)
	})

	// 启动HTTP服务器
	go func() {
		err := http.ListenAndServe(":"+port, nil)
		if err != nil {
			panic("ListenAndServe: " + err.Error())
		}
		//log.Println("ws服务启动成功")
	}()
	log.Println("wss服务启动成功,链接地址为:ws://127.0.0.1:" + port + "/ws\n" + "直播地址:" + room)

	d, _ := douyinlive.NewDouyinLive(room)

	d.Subscribe(Subscribe)
	d.Start()
}
func Subscribe(eventData *douyin.Message) {
	var marshal []byte

	msg, err := utils.MatchMethod(eventData.Method)
	if err != nil {
		if unknown == true {
			log.Println("本条消息.暂时没有源pb.无法处理.", err, hex.EncodeToString(eventData.Payload))
		}
	}
	if msg != nil {
		err := proto.Unmarshal(eventData.Payload, msg)
		if err != nil {
			log.Println("unmarshal:", err, eventData.Method)
			return
		}
		marshal, err = protojson.Marshal(msg)
		if err != nil {
			log.Println("protojson:unmarshal:", err)
			return
		}
		for _, conn := range agentlist {
			//if conn.IsClientConn

			if err := conn.WriteMessage(websocket.TextMessage, marshal); err != nil {
				log.Println("发送消息失败:", err)
				continue
			}
		}
	}

}
func serveWs(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer conn.Close()
	sec := r.Header.Get("Sec-WebSocket-Key")
	mu.Lock()
	agentlist[sec] = conn
	mu.Unlock()
	log.Println("当前连接数", len(agentlist))
	defer func() {
		mu.Lock()
		log.Println(sec, "断开连接")
		delete(agentlist, sec)
		mu.Unlock()
	}()

	// 处理WebSocket消息
	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		if string(message) == "ping" {
			if err := conn.WriteMessage(mt, []byte("pong")); err != nil {
				log.Println("write:", err)
				break
			}
		}
		// 回显消息

	}
}
