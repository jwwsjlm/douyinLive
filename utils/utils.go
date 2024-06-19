package utils

import (
	"DouyinLive/generated/douyin"
	"crypto/rand"
	"errors"
	"google.golang.org/protobuf/reflect/protoreflect"
	"math/big"
)

func GenerateMsToken(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+="

	b := make([]byte, length)
	for i := 0; i < length; i++ {
		// 生成0到charset长度之间的随机数
		randInt, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))

		// 将随机数转换为字符集中的字符
		b[i] = charset[randInt.Int64()]
	}

	return string(b) + "=_"
}
func MatchMethod(Method string) (protoreflect.ProtoMessage, error) {
	switch Method {
	case "WebcastChatMessage":

		return &douyin.ChatMessage{}, nil

	case "WebcastGiftMessage":
		return &douyin.GiftMessage{}, nil

	case "WebcastLikeMessage":
		return &douyin.LikeMessage{}, nil
	case "WebcastMemberMessage":
		return &douyin.MemberMessage{}, nil
	case "WebcastSocialMessage":
		return &douyin.SocialMessage{}, nil
	case "WebcastRoomUserSeqMessage":
		return &douyin.RoomUserSeqMessage{}, nil
	case "WebcastFansclubMessage":
		return &douyin.FansclubMessage{}, nil
	case "WebcastControlMessage":
		return &douyin.ControlMessage{}, nil
	case "WebcastEmojiChatMessage":
		return &douyin.EmojiChatMessage{}, nil
	case "WebcastRoomStatsMessage":
		return &douyin.RoomStatsMessage{}, nil
	case "WebcastRoomMessage":
		return &douyin.RoomMessage{}, nil
	case "WebcastRanklistHourEntranceMessage":
		return &douyin.RanklistHourEntranceMessage{}, nil
	case "WebcastRoomRankMessage":
		return &douyin.RoomRankMessage{}, nil
	case "WebcastInRoomBannerMessage":
		return &douyin.InRoomBannerMessage{}, nil
	case "WebcastRoomDataSyncMessage":
		return &douyin.RoomDataSyncMessage{}, nil
	case "WebcastLuckyBoxTempStatusMessage":
		return &douyin.LuckyBoxTempStatusMessage{}, nil
	case "WebcastDecorationModifyMethod":
		return &douyin.DecorationModifyMessage{}, nil
	case "WebcastLinkMicAudienceKtvMessage":
		return &douyin.LinkMicAudienceKtvMessage{}, nil
	case "WebcastRoomStreamAdaptationMessage":
		return &douyin.RoomStreamAdaptationMessage{}, nil
	case "WebcastQuizAudienceStatusMessage":
		return &douyin.QuizAudienceStatusMessage{}, nil
	case "WebcastHotChatMessage":
		return &douyin.HotChatMessage{}, nil
	case "WebcastHotRoomMessage":
		return &douyin.HotRoomMessage{}, nil
	case "WebcastAudioChatMessage":
		return &douyin.AudioChatMessage{}, nil
	case "WebcastRoomNotifyMessage":
		return &douyin.NotifyMessage{}, nil
	case "WebcastLuckyBoxMessage":
		return &douyin.LuckyBoxMessage{}, nil
	case "WebcastUpdateFanTicketMessage":
		return &douyin.UpdateFanTicketMessage{}, nil
	case "WebcastScreenChatMessage":
		return &douyin.ScreenChatMessage{}, nil
	case "WebcastNotifyEffectMessage":
		return &douyin.NotifyEffectMessage{}, nil
	case "WebcastBindingGiftMessage":
		return &douyin.NotifyEffectMessage_BindingGiftMessage{}, nil
	case "WebcastTempStateAreaReachMessage":
		return &douyin.TempStateAreaReachMessage{}, nil
	case "WebcastGrowthTaskMessage":
		return &douyin.GrowthTaskMessage{}, nil
	case "WebcastGameCPBaseMessage":
		return &douyin.GameCPBaseMessage{}, nil
	default:
		return nil, errors.New("未知消息:" + Method)
	}
}
