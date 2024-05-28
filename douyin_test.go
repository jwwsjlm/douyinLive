package douyinlive

import (
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"google.golang.org/protobuf/proto"
	"log"
	"testing"
)

func TestNewDouyinLive(t *testing.T) {
	d, _ := NewDouyinLive("296430937451")
	d.Subscribe(WebcastChatMessage, func(eventData *douyin.Message) {
		msg := &douyin.ChatMessage{}
		proto.Unmarshal(eventData.Payload, msg)
		log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content)
	})
	d.Subscribe(WebcastControlMessage, func(eventData *douyin.Message) {
		msg := &douyin.ControlMessage{}
		proto.Unmarshal(eventData.Payload, msg)
		if msg.Status == 3 {

			log.Println("直播已结束")
			d.Close()
		}
	})

	d.Start()
	select {}
}
