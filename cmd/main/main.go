package main

import (
	"encoding/hex"
	"fmt"
	"github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/utils"
	"github.com/lxzan/gws"
	"github.com/spf13/cast"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	agentlist sync.Map
	unknown   bool
	port      string
	room      string
	key       string
)

func initConfig() {
	// 设置配置文件名称和路径
	viper.SetConfigName("config")     // 配置文件名称（不带扩展名）
	viper.SetConfigType("yaml")       // 配置文件类型
	viper.AddConfigPath(".")          // 当前目录
	viper.AddConfigPath("$HOME/.app") // 家目录下的.app目录
	viper.AddConfigPath("/etc/app/")  // 系统配置目录

	// 环境变量支持
	viper.SetEnvPrefix("APP")                              // 环境变量前缀
	viper.AutomaticEnv()                                   // 自动绑定环境变量
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // 替换环境变量中的点为下划线

	// 设置默认值
	viper.SetDefault("port", "1088")
	viper.SetDefault("room", "****")
	viper.SetDefault("unknown", false)
	viper.SetDefault("key", "")

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// 配置文件不存在，使用默认值或命令行参数
			log.Println("配置文件未找到，使用默认值或命令行参数")
		} else {
			// 配置文件存在但格式错误
			log.Fatalf("配置文件解析错误: %v", err)
		}
	} else {
		log.Printf("使用配置文件: %s", viper.ConfigFileUsed())
	}
}

func main() {
	// 设置日志输出到标准输出
	log.SetOutput(os.Stdout)
	// 初始化配置
	initConfig()

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
			log.Fatalf("无法读取指定的配置文件: %v", err)
		}
		log.Printf("使用指定配置文件: %s", configFile)
	}

	// 将命令行参数绑定到viper
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		log.Fatalf("绑定命令行参数失败: %v", err)
	}
	// 获取最终配置值（命令行参数优先）
	port = viper.GetString("port")
	room = viper.GetString("room")
	unknown = viper.GetBool("unknown")
	key = viper.GetString("key")
	// 创建 gws 的 Upgrader
	upgrader := gws.NewUpgrader(&WsHandler{}, &gws.ServerOption{
		ParallelEnabled: true,         // 开启并行消息处理
		Recovery:        gws.Recovery, // 开启异常恢复
		PermessageDeflate: gws.PermessageDeflate{
			Enabled: true,
		}, //开启压缩
	})

	// 设置 WebSocket 路由
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r)

		if err != nil {
			log.Printf("升级 WebSocket 失败: %v \n 请求头:%v \n 客户端ip:%v\n", err, r.Header, r.RemoteAddr)
			return
		}
		go func() {
			socket.ReadLoop()
		}()
	})

	// 启动 WebSocket 服务器
	p := startServer(cast.ToInt(port))
	log.Printf("WebSocket 服务启动成功，地址为: ws://127.0.0.1:%s", p)

	// 创建 DouyinLive 实例
	d, err := douyinLive.NewDouyinLive(room)
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

	//log.Printf("服务器成功启动在端口 %d\n", port)
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
func Subscribe(eventData *new_douyin.Webcast_Im_Message) {
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

		RangeConnections(func(agentID string, conn *gws.Conn) {
			if err := conn.WriteString(string(marshal)); err != nil {
				log.Printf("发送消息到客户端 %s 失败: %v\n", agentID, err)
			}
		})
	}
}

// StoreConnection 储存 WebSocket 客户端连接
func StoreConnection(agentID string, conn *gws.Conn) {
	agentlist.Store(agentID, conn)
}

// GetConnection 获取 WebSocket 客户端连接
func GetConnection(agentID string) (*gws.Conn, bool) {
	value, ok := agentlist.Load(agentID)
	if !ok {
		return nil, false
	}
	conn, ok := value.(*gws.Conn)
	return conn, ok
}

// DeleteConnection 删除 WebSocket 客户端连接
func DeleteConnection(agentID string) {
	agentlist.Delete(agentID)
}

// RangeConnections 遍历 WebSocket 客户端连接
func RangeConnections(f func(agentID string, conn *gws.Conn)) {
	agentlist.Range(func(key, value interface{}) bool {
		agentID, ok := key.(string)
		if !ok {
			return true // 跳过错误的键类型
		}
		conn, ok := value.(*gws.Conn)
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

// WsHandler 实现 gws 的 Event 接口
type WsHandler struct {
	gws.BuiltinEventHandler
}

func (c *WsHandler) OnOpen(socket *gws.Conn) {
	client := socket.RemoteAddr().String()
	StoreConnection(client, socket)
	log.Printf("当前连接数: %d\n", GetConnectionCount())
}

func (c *WsHandler) OnClose(socket *gws.Conn, err error) {

	client := socket.RemoteAddr().String()
	log.Printf("客户端 %s 断开连接\n", client)
	DeleteConnection(client)
}

// OnMessage 收到消息
func (c *WsHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	//sec := socket.Request().Header.Get("Sec-WebSocket-Key")
	//log.Printf("收到消息: %s\n", sec)
	msgStr := string(message.Bytes())
	log.Printf("收到消息: %s\n", msgStr)
	if msgStr == "ping" {
		if err := socket.WriteString("pong"); err != nil {
			log.Printf("写入消息失败: %v\n", err)
		}
	}
}
