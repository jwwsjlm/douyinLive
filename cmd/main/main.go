package main

import (
	"encoding/hex"
	"encoding/json"
	"github.com/jwwsjlm/douyinLive/generated"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/lxzan/gws"
	"github.com/spf13/cast"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

var (
	agentlist  sync.Map // 存储所有连接，键为客户端ID，值为连接对象
	unknown    bool
	port       string
	room       string // 抖音直播房间号
	key        string
	logger     *log.Logger
	wsHandler  WsHandler // 创建 WebSocket 处理器实例
	roomGroups sync.Map
	//groupRooms *gws.ConcurrentMap[string, *gws.Conn]
)

func cmdArgs() {
	// 定义命令行参数
	pflag.String("port", viper.GetString("port"), "WebSocket 服务端口")
	pflag.String("room", viper.GetString("room"), "抖音直播房间号")
	pflag.Bool("unknown", viper.GetBool("unknown"), "是否输出未知源的pb消息")
	pflag.String("key", viper.GetString("key"), "tikhub key")
	configFile := *pflag.String("config", "", "指定配置文件路径")
	// 解析命令行参数
	pflag.Parse()

	// 如果指定了配置文件，则使用该配置文件
	if configFile != "" {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			logger.Fatalf("无法读取指定的配置文件: %v", err)
		}
		logger.Printf("使用指定配置文件: %s", configFile)
	}

	// 将命令行参数绑定到viper
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		logger.Fatalf("绑定命令行参数失败: %v", err)
	}
	// 获取最终配置值（命令行参数优先）
	port = viper.GetString("port")
	room = viper.GetString("room")
	if room == "****" {
		logger.Fatal("请提供抖音直播房间号")
	}
	unknown = viper.GetBool("unknown")
	key = viper.GetString("key")
}
func main() {
	logger = log.Default()
	logger.SetOutput(os.Stdout)
	initConfig()
	cmdArgs()

	// 设置动态WebSocket路由，匹配 /ws/ 开头的所有路径
	http.HandleFunc("/ws/", routing)

	// 启动 WebSocket 服务器
	p := startServer(cast.ToInt(port))
	logger.Printf("WebSocket 服务启动成功，地址为: ws://127.0.0.1:%s", p)
	// 阻塞直到接收到终止信号
	// 等待终止信号
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	logger.Println("接收到终止信号，开始优雅关闭...")

}

// routing 处理 WebSocket 连接请求
func routing(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Path[4:] // 截取 /ws/ 之后的部分作为房间ID
	if roomID == "" {
		http.Error(w, "无效的房间ID", http.StatusBadRequest)
		return
	}

	logger.Printf("接收到WebSocket连接请求，房间ID: %s，客户端: %s", roomID, r.RemoteAddr)

	// 创建带有房间ID信息的处理器

	handler := &WsHandler{RoomID: roomID}
	upgrader := gws.NewUpgrader(handler, &gws.ServerOption{
		ParallelEnabled: true,
		Recovery:        gws.Recovery,
		PermessageDeflate: gws.PermessageDeflate{
			Enabled: true,
		},
	})

	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		logger.Printf("升级失败: %v", err)
		return
	}

	// 启动读取循环
	go socket.ReadLoop()
}

// startServer 启动 WebSocket 服务端
func startServer(port int) string {
	for {
		if checkPortAvailability(port) {
			go func() {
				if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
					logger.Fatalf("服务器启动失败: %v", err)
				}
			}()
			break
		}
		port++ // 如果端口被占用，增加端口号
	}

	//log.Printf("服务器成功启动在端口 %d\n", port)
	return strconv.Itoa(port)
}

// routeEventToRoomClients 将抖音直播事件路由到特定房间的所有客户端
func routeEventToRoomClients(roomID string, eventData *new_douyin.Webcast_Im_Message, n string) {
	msg, err := generated.GetMessageInstance(eventData.Method)
	if err != nil {
		if unknown {
			logger.Printf("未知消息，无法处理: %v, %s\n", err, hex.EncodeToString(eventData.Payload))
		}
		return
	}

	if msg != nil {
		if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
			logger.Printf("反序列化失败: %v, 方法: %s\n", err, eventData.Method)
			return
		}

		marshal, err := protojson.Marshal(msg)
		if err != nil {
			logger.Printf("JSON 序列化失败: %v\n", err)
			return
		}
		// 解析JSON到动态map
		var msgMap map[string]interface{}
		if err := json.Unmarshal(marshal, &msgMap); err != nil {
			logger.Printf("解析JSON失败: %v\n", err)
			return
		}

		msgMap["livename"] = n // 添加直播名称到消息中
		// 重新序列化
		finalJSON, err := json.Marshal(msgMap)
		if err != nil {
			logger.Printf("包装消息序列化失败: %v\n", err)
			return
		}
		// 获取房间组
		if group, ok := roomGroups.Load(roomID); ok {
			roomGroup := group.(*RoomGroup)

			//将消息广播到房间内所有客户端
			broadcastMessage(roomGroup.connections, roomID, finalJSON)

		}
	}
}

// broadcastMessage 将消息广播到房间内所有客户端
func broadcastMessage(roomGroup *gws.ConcurrentMap[string, *gws.Conn], roomID string, message []byte) {
	roomGroup.Range(func(clientID string, conn *gws.Conn) bool {
		if conn != nil {
			if err := conn.WriteMessage(gws.OpcodeText, message); err != nil {
				logger.Printf("发送消息到客户端 %s (房间: %s) 失败: %v\n", clientID, roomID, err)
			} else {
				//logger.Printf("向客户端 %s (房间: %s) 发送消息成功\n", clientID, roomID)
			}
		} else {
			logger.Printf("客户端 %s (房间: %s) 已断开连接，无法发送消息\n", clientID, roomID)
		}
		return true // 继续遍历

	})
}
func NewWebSocket() *WsHandler {
	return &WsHandler{
		sessions: gws.NewConcurrentMap[string, *gws.Conn](16, 128),
	}
}
func MustLoad[T any](session gws.SessionStorage, key string) (v T) {
	if value, exist := session.Load(key); exist {
		v, _ = value.(T)
	}
	return
}
