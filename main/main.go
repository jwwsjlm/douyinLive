package main

import (
	"flag"
	"github.com/gorilla/websocket"
	douyinlive "github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
)

var addr = flag.String("addr", ":18080", "http service address")
var agentlist = make([]*websocket.Conn, 0)

func main() {
	flag.Parse()
	log.SetFlags(0)

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
	go http.ListenAndServe(*addr, nil)
	d, _ := douyinlive.NewDouyinLive("933572413882")

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
			//d.Emit(Default, data.Payload)
			//log.Println("payload:", method, hex.EncodeToString(data.Payload))
		}
		if msg != nil {
			err := proto.Unmarshal(eventData.Payload, msg)
			if err != nil {
				log.Println("unmarshal:", err)
				return
			}
			marshal, _ = protojson.Marshal(msg)
			for _, conn := range agentlist {
				if err := conn.WriteMessage(websocket.TextMessage, marshal); err != nil {
					log.Println("write:", err)
					break
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
	agentlist = append(agentlist, conn)
	// 处理WebSocket消息
	for {
		mt, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			continue
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
