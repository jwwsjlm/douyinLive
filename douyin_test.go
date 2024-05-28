package douyinlive

import (
	"github.com/jwwsjlm/douyinLive/generated/douyin"
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
			//msg.String()
			log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content, string(marshal), msg.String())
		}
		//log.Println(eventData.Method, string(eventData.Payload))
		//msg := &douyin.ChatMessage{}
		//
		//proto.Unmarshal(eventData.Payload, msg)
		//marshal, _ := protojson.Marshal(msg)
		////msg.String()
		//log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content, string(marshal), msg.String())
	})

	d.Start()
	select {}
}
