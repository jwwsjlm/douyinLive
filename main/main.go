package main

import (
	"encoding/hex"
	"github.com/gorilla/websocket"
	douyinlive "github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"strconv"
	"sync"
)

var agentlist = make(map[string]*websocket.Conn)
var mu sync.Mutex // 使用互斥锁来保护用户列表
func main() {
	var port int
	pflag.IntVar(&port, "port", 18080, "ws端口")
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
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(upgrader, w, r)
	})

	// 启动HTTP服务器
	go func() {
		err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
		if err != nil {
			panic("ListenAndServe: " + err.Error())
		}
		//log.Println("ws服务启动成功")
	}()
	log.Println("wss服务启动成功")
	d, _ := douyinlive.NewDouyinLive(room)

	d.Subscribe(func(eventData *douyin.Message) {
		var marshal []byte
		var msg proto.Message
		switch eventData.Method {

		case "WebcastChatMessage":
			msg = &douyin.ChatMessage{}
			//proto.Unmarshal(eventData.Payload, msg)
			//marshal, _ = protojson.Marshal(msg)
		case "WebcastGiftMessage":
			msg = &douyin.GiftMessage{}
			//proto.Unmarshal(eventData.Payload, msg)
			//marshal, _ = protojson.Marshal(msg)
		case "WebcastLikeMessage":
			msg = &douyin.LikeMessage{}
			//proto.Unmarshal(eventData.Payload, msg)
			//marshal, _ = protojson.Marshal(msg)
		case "WebcastMemberMessage":

			msg = &douyin.MemberMessage{}
			//proto.Unmarshal(eventData.Payload, msg)
			//marshal, _ = protojson.Marshal(msg)
		case "WebcastSocialMessage":
			msg = &douyin.SocialMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Println("关注msg", msg.User.Id, msg.User.NickName)
		case "WebcastRoomUserSeqMessage":
			msg = &douyin.RoomUserSeqMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("房间人数msg 当前观看人数:%v,累计观看人数:%v\n", msg.Total, msg.TotalPvForAnchor)
		case "WebcastFansclubMessage":
			msg = &douyin.FansclubMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("粉丝团msg %v\n", msg.Content)
		case "WebcastControlMessage":
			msg = &douyin.ControlMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("直播间状态消息%v", msg.Status)
		case "WebcastEmojiChatMessage":
			msg = &douyin.EmojiChatMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("表情消息%vuser:%vcommon:%vdefault_content:%v", msg.EmojiId, msg.User, msg.Common, msg.DefaultContent)
		case "WebcastRoomStatsMessage":
			msg = &douyin.RoomStatsMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("直播间统计msg%v", msg.DisplayLong)
		case "WebcastRoomMessage":
			msg = &douyin.RoomMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("【直播间msg】直播间id%v", msg.Common.RoomId)
		case "WebcastRoomRankMessage":
			msg = &douyin.RoomRankMessage{}
			//proto.Unmarshal(data.Payload, msg)
			//log.Printf("直播间排行榜msg%v", msg.RanksList)

		default:
			if unknown == true {
				log.Println("本条消息.暂时没有源pb.无法处理.", hex.EncodeToString(eventData.Payload))
			}
			//d.Emit(Default, data.Payload)
			//log.Println("payload:", method, hex.EncodeToString(data.Payload))
		}
		if msg != nil {
			err := proto.Unmarshal(eventData.Payload, msg)
			if err != nil {
				log.Println("unmarshal:", err)
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

	})
	d.Start()
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
