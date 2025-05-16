package generated

import (
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var MessageMap = map[string]func() protoreflect.ProtoMessage{
	"WebcastChatMessage":                 func() protoreflect.ProtoMessage { return &douyin.ChatMessage{} },
	"WebcastGiftMessage":                 func() protoreflect.ProtoMessage { return &douyin.GiftMessage{} },
	"WebcastLikeMessage":                 func() protoreflect.ProtoMessage { return &douyin.LikeMessage{} },
	"WebcastMemberMessage":               func() protoreflect.ProtoMessage { return &douyin.MemberMessage{} },
	"WebcastSocialMessage":               func() protoreflect.ProtoMessage { return &douyin.SocialMessage{} },
	"WebcastRoomUserSeqMessage":          func() protoreflect.ProtoMessage { return &douyin.RoomUserSeqMessage{} },
	"WebcastFansclubMessage":             func() protoreflect.ProtoMessage { return &douyin.FansclubMessage{} },
	"WebcastControlMessage":              func() protoreflect.ProtoMessage { return &douyin.ControlMessage{} },
	"WebcastEmojiChatMessage":            func() protoreflect.ProtoMessage { return &douyin.EmojiChatMessage{} },
	"WebcastRoomStatsMessage":            func() protoreflect.ProtoMessage { return &douyin.RoomStatsMessage{} },
	"WebcastRoomMessage":                 func() protoreflect.ProtoMessage { return &douyin.RoomMessage{} },
	"WebcastRanklistHourEntranceMessage": func() protoreflect.ProtoMessage { return &douyin.RanklistHourEntranceMessage{} },
	"WebcastRoomRankMessage":             func() protoreflect.ProtoMessage { return &douyin.RoomRankMessage{} },
	"WebcastInRoomBannerMessage":         func() protoreflect.ProtoMessage { return &douyin.InRoomBannerMessage{} },
	"WebcastRoomDataSyncMessage":         func() protoreflect.ProtoMessage { return &douyin.RoomDataSyncMessage{} },
	"WebcastLuckyBoxTempStatusMessage":   func() protoreflect.ProtoMessage { return &douyin.LuckyBoxTempStatusMessage{} },
	"WebcastDecorationModifyMethod":      func() protoreflect.ProtoMessage { return &douyin.DecorationModifyMessage{} },
	"WebcastLinkMicAudienceKtvMessage":   func() protoreflect.ProtoMessage { return &douyin.LinkMicAudienceKtvMessage{} },
	"WebcastRoomStreamAdaptationMessage": func() protoreflect.ProtoMessage { return &douyin.RoomStreamAdaptationMessage{} },
	"WebcastQuizAudienceStatusMessage":   func() protoreflect.ProtoMessage { return &douyin.QuizAudienceStatusMessage{} },
	"WebcastHotChatMessage":              func() protoreflect.ProtoMessage { return &douyin.HotChatMessage{} },
	"WebcastHotRoomMessage":              func() protoreflect.ProtoMessage { return &douyin.HotRoomMessage{} },
	"WebcastAudioChatMessage":            func() protoreflect.ProtoMessage { return &douyin.AudioChatMessage{} },
	"WebcastRoomNotifyMessage":           func() protoreflect.ProtoMessage { return &douyin.NotifyMessage{} },
	"WebcastLuckyBoxMessage":             func() protoreflect.ProtoMessage { return &douyin.LuckyBoxMessage{} },
	"WebcastUpdateFanTicketMessage":      func() protoreflect.ProtoMessage { return &douyin.UpdateFanTicketMessage{} },
	"WebcastScreenChatMessage":           func() protoreflect.ProtoMessage { return &douyin.ScreenChatMessage{} },
	"WebcastNotifyEffectMessage":         func() protoreflect.ProtoMessage { return &douyin.NotifyEffectMessage{} },
	"WebcastBindingGiftMessage":          func() protoreflect.ProtoMessage { return &douyin.NotifyEffectMessage_BindingGiftMessage{} },
	"WebcastTempStateAreaReachMessage":   func() protoreflect.ProtoMessage { return &douyin.TempStateAreaReachMessage{} },
	"WebcastGrowthTaskMessage":           func() protoreflect.ProtoMessage { return &douyin.GrowthTaskMessage{} },
	"WebcastGameCPBaseMessage":           func() protoreflect.ProtoMessage { return &douyin.GameCPBaseMessage{} },
	"WebcastSandwichBorderMessage":       func() protoreflect.ProtoMessage { return &douyin.SandwichBorderMessage{} },
	"WebcastLiveEcomGeneralMessage": func() protoreflect.ProtoMessage {
		return &douyin.LiveEcomGeneralMessage{}
	},
	"WebcastLiveShoppingMessage": func() protoreflect.ProtoMessage {
		return &douyin.LiveShoppingMessage{}
	},
	//"WebcastChatLikeMessage": func() protoreflect.ProtoMessage {
	//	return &douyin.ChatLikeMessage{}
	//},
}

// NewMessage 匹配抖音直播消息
var NewMessage = map[string]func() protoreflect.ProtoMessage{
	"WebcastChatMessage":                 func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ChatMessage{} },
	"WebcastGiftMessage":                 func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GiftMessage{} },
	"WebcastLikeMessage":                 func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LikeMessage{} },
	"WebcastMemberMessage":               func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_MemberMessage{} },
	"WebcastSocialMessage":               func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SocialMessage{} },
	"WebcastRoomUserSeqMessage":          func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomUserSeqMessage{} },
	"WebcastFansclubMessage":             func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_FansclubMessage{} },
	"WebcastControlMessage":              func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ControlMessage{} },
	"WebcastEmojiChatMessage":            func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_EmojiChatMessage{} },
	"WebcastRoomStatsMessage":            func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStatsMessage{} },
	"WebcastRoomMessage":                 func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomMessage{} },
	"WebcastRanklistHourEntranceMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RanklistHourEntranceMessage{} },
	"WebcastRoomRankMessage":             func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomRankMessage{} },
	"WebcastInRoomBannerMessage":         func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_InRoomBannerMessage{} },
	"WebcastRoomDataSyncMessage":         func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomDataSyncMessage{} },
	"WebcastLuckyBoxTempStatusMessage":   func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxTempStatusMessage{} },
	"WebcastDecorationModifyMethod":      func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_DecorationModifyMessage{} },
	"WebcastLinkMicAudienceKtvMessage":   func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LinkMicAudienceKtvMessage{} },
	"WebcastRoomStreamAdaptationMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStreamAdaptationMessage{} },
	"WebcastQuizAudienceStatusMessage":   func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_QuizAudienceStatusMessage{} },
	"WebcastHotChatMessage":              func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotChatMessage{} },
	"WebcastHotRoomMessage":              func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotRoomMessage{} },
	"WebcastAudioChatMessage":            func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_AudioChatMessage{} },
	"WebcastRoomNotifyMessage":           func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_MediaRoomNoticeMessage{} },
	"WebcastLuckyBoxMessage":             func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxMessage{} },
	"WebcastUpdateFanTicketMessage":      func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_UpdateFanTicketMessage{} },
	"WebcastScreenChatMessage":           func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ScreenChatMessage{} },
	"WebcastNotifyEffectMessage":         func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NotifyEffectMessage{} },
	"WebcastBindingGiftMessage":          func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_BindingGiftMessage{} },
	"WebcastTempStateAreaReachMessage":   func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_TempStateAreaReachMessage_Resource{} },
	"WebcastGrowthTaskMessage":           func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GrowthTaskMessage{} },
	"WebcastGameCPBaseMessage":           func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameCPBaseMessage{} },
	"WebcastSandwichBorderMessage":       func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SandwichBorderMessage{} },
	"WebcastLiveEcomGeneralMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveEcomGeneralMessage{}
	},
	"WebcastLiveShoppingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveShoppingMessage{}
	},
	"WebcastChatLikeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ChatLikeMessage{}
	},
	"WebcastLinkmicOrderSingScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingScoreMessage{}
	},
	"WebcastLinkerContributeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkerContributeMessage{}
	},
	"WebcastLinkMicSendEmojiMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicSendEmojiMessage{}
	},
	"WebcastLinkmicPlaymodeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlaymodeMessage{}
	},
	"WebcastActivityEmojiGroupsMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ActivityEmojiGroupsMessage{}
	},
	"WebcastLinkmicPlayModeUpdateScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlayModeUpdateScoreMessage{}
	},
	"WebcastLinkMicMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicMethod{}
	},
	"WebcastLinkMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMessage{}
	},
	"WebcastLinkmicOrderSingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingMessage{}
	},
	"WebcastLinkMicPositionMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicPositionMessage{}
	},
	"WebcastLinkmicEnlargeGuestMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicEnlargeGuestMessage{}
	},
	"WebcastLinkmicProfitMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicProfitMessage{}
	},
	"WebcastGiftIconFlashMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftIconFlashMessage{}
	},
	"WebcastGiftSortMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftSortMessage{}
	},
	"WebcastPrivilegeScreenChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_PrivilegeScreenChatMessage{}
	},
	"WebcastExhibitionChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ExhibitionChatMessage{}
	},
	"WebcastShelfTradeDataMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ShelfTradeDataMessage{}
	},
	"WebcastVisibilityRangeChangeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_VisibilityRangeChangeMessage{}
	},
	"WebcastFeedbackCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_FeedbackCardMessage{}
	},
	"WebcastStampMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_StampMessage{}
	},
	"WebcastCustomizedCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_CustomizedCardMessage{}
	},
	"WebcastAudienceEntranceMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_AudienceEntranceMessage{}
	},
	"WebcastRoomIntroMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomIntroMessage{} },
}
