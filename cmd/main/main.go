package main

import (
	"douyinlive"
	"douyinlive/generated/douyin"
	"douyinlive/utils"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/spf13/cast"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	agentlist sync.Map
	unknown   bool
)

func main() {
	var port string
	var room string
	pflag.StringVar(&port, "port", "18080", "WebSocket 服务端口")
	pflag.StringVar(&room, "room", "****", "抖音直播房间号")
	pflag.BoolVar(&unknown, "unknown", false, "是否输出未知源的pb消息")
	pflag.Parse()

	// 创建 WebSocket 升级器
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有 CORS 请求，实际应用中应根据需要设置
		},
	}

	// 设置 WebSocket 路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveWs(upgrader, w, r)
	})

	// 启动 WebSocket 服务器
	p := startServer(cast.ToInt(port))
	log.Printf("WebSocket 服务启动成功，地址为: ws://127.0.0.1:%s/\n直播房间: %s\n", p, room)

	// 创建 DouyinLive 实例
	d, err := douyinlive.NewDouyinLive(room)
	if err != nil {
		log.Fatalf("抖音链接失败: %v", err)
	}

	// 订阅事件
	d.Subscribe(Subscribe)
	// 开始处理
	d.Start()
}

// startServer 启动 WebSocket 服务端
func startServer(port int) string {
	for {
		if checkPortAvailability(port) {
			go func() {
				if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
					log.Fatalf("服务器启动失败: %v", err)
				}
			}()
			break
		}
		port++ // 如果端口被占用，增加端口号
	}

	log.Printf("服务器成功启动在端口 %d\n", port)
	return strconv.Itoa(port)
}

// checkPortAvailability 检查本地端口是否可用
func checkPortAvailability(port int) bool {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true // 如果连接失败，认为端口可用
	}
	conn.Close()
	return false // 如果连接成功，认为端口不可用
}

// Subscribe 处理订阅的更新
func Subscribe(eventData *douyin.Message) {
	msg, err := utils.MatchMethod(eventData.Method)
	if err != nil {
		if unknown {
			log.Printf("未知消息，无法处理: %v, %s\n", err, hex.EncodeToString(eventData.Payload))
		}
		return
	}

	if msg != nil {
		if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
			log.Printf("反序列化失败: %v, 方法: %s\n", err, eventData.Method)
			return
		}

		marshal, err := protojson.Marshal(msg)
		if err != nil {
			log.Printf("JSON 序列化失败: %v\n", err)
			return
		}

		RangeConnections(func(agentID string, conn *websocket.Conn) {
			if err := conn.WriteMessage(websocket.TextMessage, marshal); err != nil {
				log.Printf("发送消息到客户端 %s 失败: %v\n", agentID, err)
			}
		})
	}
}

// StoreConnection 储存 WebSocket 客户端连接
func StoreConnection(agentID string, conn *websocket.Conn) {
	agentlist.Store(agentID, conn)
}

// GetConnection 获取 WebSocket 客户端连接
func GetConnection(agentID string) (*websocket.Conn, bool) {
	value, ok := agentlist.Load(agentID)
	if !ok {
		return nil, false
	}
	conn, ok := value.(*websocket.Conn)
	return conn, ok
}

// DeleteConnection 删除 WebSocket 客户端连接
func DeleteConnection(agentID string) {
	agentlist.Delete(agentID)
}

// RangeConnections 遍历 WebSocket 客户端连接
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

// GetConnectionCount 获取当前连接数
func GetConnectionCount() int {
	count := 0
	agentlist.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// serveWs 处理 WebSocket 请求
func serveWs(upgrader websocket.Upgrader, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("升级 WebSocket 失败: %v\n", err)
		return
	}
	defer conn.Close()

	sec := r.Header.Get("Sec-WebSocket-Key")
	StoreConnection(sec, conn)
	log.Printf("当前连接数: %d\n", GetConnectionCount())

	defer func() {
		log.Printf("客户端 %s 断开连接\n", sec)
		DeleteConnection(sec)
	}()

	// 处理 WebSocket 消息
	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("读取消息失败: %v\n", err)
			break
		}
		log.Printf("收到消息: %s\n", message)
		if string(message) == "ping" {
			if err := conn.WriteMessage(mt, []byte("pong")); err != nil {
				log.Printf("写入消息失败: %v\n", err)
				break
			}
		}
	}
}
