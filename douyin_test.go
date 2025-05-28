package douyinLive

import (
	"log"
	"testing"

	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func TestNewDouyinLive(t *testing.T) {
	d, _ := NewDouyinLive("740934774657", log.Default())
	d.Subscribe(func(eventData *new_douyin.Webcast_Im_Message) {
		// t.Logf("msg received ,type:%s", eventData.Method)
		switch eventData.Method {
		case WebcastChatMessage:
			msg := &new_douyin.Webcast_Im_ChatMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			t.Logf("聊天消息:user=%d %s %s", msg.User.Id, msg.User.Nickname, msg.Content)
		case WebcastGiftMessage:
			msg := &new_douyin.Webcast_Im_GiftMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			t.Logf("礼物消息:user=%d %s %s", msg.User.Id, msg.User.Nickname, msg.Gift.Name)
		case WebcastLikeMessage:
			msg := &new_douyin.Webcast_Im_LikeMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			t.Logf("点赞消息:user=%d %s", msg.User.Id, msg.User.Nickname)
		case WebcastMemberMessage:
			msg := &new_douyin.Webcast_Im_MemberMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			t.Logf("成员消息:user=%d %s", msg.User.Id, msg.User.Nickname)
		case WebcastSocialMessage:
			msg := &new_douyin.Webcast_Im_SocialMessage{}
			proto.Unmarshal(eventData.Payload, msg)
			t.Logf("社交消息:user=%d %s", msg.User.Id, msg.User.Nickname)
		default:
			t.Logf("其他消息:type:%s", eventData.Method)
		}

		// if eventData.Method == douyinlive.WebcastChatMessage {
		// 	msg := &douyin.ChatMessage{}
		// 	proto.Unmarshal(eventData.Payload, msg)
		// 	marshal, _ := protojson.Marshal(msg)
		// 	log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content, string(marshal))
		// }
	})

	d.Start()

}

func TestConstructWSSURL(t *testing.T) {
	d, err := NewDouyinLive("483379663830", log.Default())
	if err != nil {
		t.Fatalf("创建 DouyinLive 实例失败: %v", err)
	}
	wssURL, err := d.makeURL()
	if err != nil {
		t.Fatalf("构建 WSS URL 失败: %v", err)
	}
	t.Logf("构建的 WSS URL: %s", wssURL)
}
