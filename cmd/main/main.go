package main

import (
	"douyinLive"
	"douyinLive/generated/douyin"
	"douyinLive/utils"
	"encoding/hex"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/spf13/cast"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
)

var agentlist sync.Map
var unknown bool

func main() {
	var port string
	pflag.StringVar(&port, "port", "18080", "ws端口")
	var room string
	pflag.StringVar(&room, "room", "****", "房间号")
	var unknown bool
	pflag.BoolVar(&unknown, "unknown", false, "未知源pb消息是否输出")
	pflag.Parse()
	// 创建WebSocket升级器
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有CORS请求，实际应用中应根据需要设置
		},
	}
	// 设置WebSocket路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(upgrader, w, r)
	})
	p := startServer(cast.ToInt(port))
	log.Println("wss服务启动成功,链接地址为:ws://127.0.0.1:" + p + "/\n" + "直播地址:" + room)
	d, err := douyinlive.NewDouyinLive(room)
	if err != nil {
		panic("抖音链接失败:" + err.Error())
	}

	d.Subscribe(Subscribe)
	//开始
	d.Start()
}

// startServer 启动ws服务端
func startServer(port int) string {
	for { // 一直循环，每次端口+1
		if isPortAvailable(port) {
			go func() {
				if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
					panic(err)
				}
			}()
			break
		} else {
			port++ // 如果端口被占用，增加端口号
		}
	}

	log.Printf("服务器成功启动在端口 %d\n", port)
	return cast.ToString(port)
}

// isPortAvailable 判断端口是否被占用
func isPortAvailable(port int) bool {
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return true // 端口可能被占用
	}
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			panic(err)
		}
	}(conn)
	return false // 端口未被占用
}

// Subscribe 订阅更新
func Subscribe(eventData *douyin.Message) {
	var marshal []byte
	msg, err := utils.MatchMethod(eventData.Method)
	if err != nil {
		if unknown == true {
			log.Println("本条消息.暂时没有源pb.无法处理.", err, hex.EncodeToString(eventData.Payload))
			return
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
		RangeConnections(func(agentID string, conn *websocket.Conn) {
			err := conn.WriteMessage(websocket.TextMessage, marshal)
			if err != nil {
				log.Println("Error sending message to agent", agentID, ":", err)
			}
		})

	}

}

// StoreConnection 储存ws客户
func StoreConnection(agentID string, conn *websocket.Conn) {
	agentlist.Store(agentID, conn)
}

// GetConnection 获取一个链接
func GetConnection(agentID string) (*websocket.Conn, bool) {
	value, ok := agentlist.Load(agentID)
	if !ok {
		return nil, false
	}
	conn, ok := value.(*websocket.Conn) // 类型断言
	return conn, ok
}

// DeleteConnection 删除一个ws客户
func DeleteConnection(agentID string) {
	agentlist.Delete(agentID)
}

// RangeConnections 遍历ws客户端
func RangeConnections(f func(agentID string, conn *websocket.Conn)) {
	agentlist.Range(func(key, value interface{}) bool {
		agentID, ok := key.(string)
		if !ok {
			return true // 跳过错误的键类型
		}
		conn, ok := value.(*websocket.Conn)
		if !ok {
			return true // 跳过错误的值类型
		}
		f(agentID, conn)
		return true
	})
}
func GetConnectionCount() int {
	count := 0
	agentlist.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// serveWs 处理ws请求
func serveWs(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	defer func(conn *websocket.Conn) {
		err := conn.Close()
		if err != nil {
			log.Println(err)
		}
	}(conn)
	sec := r.Header.Get("Sec-WebSocket-Key")
	StoreConnection(sec, conn)
	log.Println("当前连接数", GetConnectionCount())
	defer func() {
		log.Println(sec, "断开连接")
		DeleteConnection(sec)
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
