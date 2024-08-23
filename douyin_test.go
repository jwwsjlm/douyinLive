package douyinlive

import (
	"douyinlive/generated/douyin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"log"
	"testing"
)

func TestNewDouyinLive(t *testing.T) {
	d, _ := NewDouyinLive("644826113301")
	d.Subscribe(func(eventData *douyin.Message) {
		if eventData.Method == WebcastChatMessage {
			msg := &douyin.ChatMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			marshal, _ := protojson.Marshal(msg)
			log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content, string(marshal))
		}
	})

	d.Start()

}
