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
	// 直播文字聊天消息（普通文本弹幕）
	"WebcastChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ChatMessage{} },

	// 直播礼物赠送消息（用户送礼行为通知）
	"WebcastGiftMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GiftMessage{} },

	// 直播点赞消息（用户点击直播间点赞按钮）
	"WebcastLikeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LikeMessage{} },

	// 直播间成员变动消息（加入/离开/关注等）
	"WebcastMemberMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_MemberMessage{} },

	// 直播社交互动消息（分享/关注/邀请等社交行为）
	"WebcastSocialMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SocialMessage{} },

	// 直播间用户序列消息（维护在线用户列表顺序）
	"WebcastRoomUserSeqMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomUserSeqMessage{} },

	// 粉丝团相关消息（入团、升级、粉丝任务通知）
	"WebcastFansclubMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_FansclubMessage{} },

	// 直播控制消息（禁言、清屏、设置管理员等操作）
	"WebcastControlMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ControlMessage{} },

	// 表情聊天消息（带Emoji表情的弹幕）
	"WebcastEmojiChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_EmojiChatMessage{} },

	// 直播间统计消息（在线人数、互动数据、礼物收益）
	"WebcastRoomStatsMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStatsMessage{} },

	// 直播间通用通知消息（系统公告、活动提示）
	"WebcastRoomMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomMessage{} },

	// 小时榜入口消息（触发 hourly ranking list 显示）
	"WebcastRanklistHourEntranceMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RanklistHourEntranceMessage{} },

	// 直播间排名消息（礼物贡献榜、互动榜实时更新）
	"WebcastRoomRankMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomRankMessage{} },

	// 直播间内横幅消息（顶部滚动活动公告）
	"WebcastInRoomBannerMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_InRoomBannerMessage{} },

	// 直播间数据同步消息（多端状态一致性更新）
	"WebcastRoomDataSyncMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomDataSyncMessage{} },

	// 盲盒临时状态消息（抽奖活动进度通知）
	"WebcastLuckyBoxTempStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxTempStatusMessage{} },

	// 直播间装饰修改消息（背景、边框等视觉元素调整）
	"WebcastDecorationModifyMethod": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_DecorationModifyMessage{} },

	// 连麦观众K歌消息（多人连麦K歌场景）
	"WebcastLinkMicAudienceKtvMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LinkMicAudienceKtvMessage{} },

	// 直播流自适应消息（动态调整视频清晰度）
	"WebcastRoomStreamAdaptationMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStreamAdaptationMessage{} },

	// 问答观众状态消息（互动答题活动参与情况）
	"WebcastQuizAudienceStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_QuizAudienceStatusMessage{} },

	// 热门聊天消息（高互动量弹幕聚合展示）
	"WebcastHotChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotChatMessage{} },

	// 热门直播间推荐消息（推送同类热门房间）
	"WebcastHotRoomMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotRoomMessage{} },

	// 语音聊天消息（纯音频直播互动）
	"WebcastAudioChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_AudioChatMessage{} },

	// 直播间媒体通知消息（重要事件弹窗提示）
	"WebcastRoomNotifyMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NotifyMessage{} },

	// 盲盒活动消息（抽奖结果、奖励发放通知）
	"WebcastLuckyBoxMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxMessage{} },

	// 粉丝票更新消息（粉丝团专属积分变动）
	"WebcastUpdateFanTicketMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_UpdateFanTicketMessage{} },

	// 屏幕弹幕消息（全屏滚动特效弹幕）
	"WebcastScreenChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ScreenChatMessage{} },

	// 通知特效消息（礼物连击、关注等动画效果）
	"WebcastNotifyEffectMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NotifyEffectMessage{} },

	// 绑定礼物消息（任务奖励礼物发放）
	"WebcastBindingGiftMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_BindingGiftMessage{} },

	// 临时状态区域触发消息（如福袋领取区域进入通知）
	"WebcastTempStateAreaReachMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_TempStateAreaReachMessage_Resource{} },

	// 成长任务消息（观看时长、互动次数奖励通知）
	"WebcastGrowthTaskMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GrowthTaskMessage{} },

	// 第三方游戏基础消息（直播内嵌游戏互动）
	"WebcastGameCPBaseMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameCPBaseMessage{} },

	// 夹层边框特效消息（直播间主题装饰边框）
	"WebcastSandwichBorderMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SandwichBorderMessage{} },

	// 直播电商通用消息（商品上架、促销活动通知）
	"WebcastLiveEcomGeneralMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveEcomGeneralMessage{}
	},

	// 直播购物消息（下单、抢购、优惠券领取通知）
	"WebcastLiveShoppingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveShoppingMessage{}
	},

	// 聊天内容点赞消息（对特定弹幕的点赞互动）
	"WebcastChatLikeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ChatLikeMessage{}
	},

	// 连麦点歌评分消息（K歌直播中的演唱评分）
	"WebcastLinkmicOrderSingScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingScoreMessage{}
	},

	// 连麦贡献消息（连麦嘉宾的礼物贡献统计）
	"WebcastLinkerContributeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkerContributeMessage{}
	},

	// 连麦发送表情消息（连麦场景下的表情互动）
	"WebcastLinkMicSendEmojiMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicSendEmojiMessage{}
	},

	// 连麦播放模式消息（伴奏/原唱模式切换通知）
	"WebcastLinkmicPlaymodeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlaymodeMessage{}
	},

	// 活动表情组消息（限时活动专属表情包）
	"WebcastActivityEmojiGroupsMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ActivityEmojiGroupsMessage{}
	},

	// 连麦模式更新分数消息（实时K歌评分更新）
	"WebcastLinkmicPlayModeUpdateScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlayModeUpdateScoreMessage{}
	},

	// 连麦操作消息（申请/接受/挂断连麦请求）
	"WebcastLinkMicMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicMethod{}
	},

	// 分享链接消息（直播间分享行为通知）
	"WebcastLinkMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMessage{}
	},

	// 连麦点歌消息（观众请求演唱歌曲）
	"WebcastLinkmicOrderSingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingMessage{}
	},

	// 连麦位置调整消息（主麦/副麦布局变更）
	"WebcastLinkMicPositionMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicPositionMessage{}
	},

	// 连麦嘉宾放大消息（特写展示某连麦者画面）
	"WebcastLinkmicEnlargeGuestMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicEnlargeGuestMessage{}
	},

	// 连麦收益消息（嘉宾礼物分成比例通知）
	"WebcastLinkmicProfitMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicProfitMessage{}
	},

	// 礼物图标闪烁特效消息（高价值礼物全屏动画）
	"WebcastGiftIconFlashMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftIconFlashMessage{}
	},

	// 礼物排序消息（调整礼物列表展示优先级）
	"WebcastGiftSortMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftSortMessage{}
	},

	// 特权弹幕消息（会员专属特效弹幕）
	"WebcastPrivilegeScreenChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_PrivilegeScreenChatMessage{}
	},

	// 展示聊天消息（精选弹幕置顶显示）
	"WebcastExhibitionChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ExhibitionChatMessage{}
	},

	// 货架交易数据消息（商品实时销量/库存更新）
	"WebcastShelfTradeDataMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ShelfTradeDataMessage{}
	},

	// 可见范围变更消息（直播内容地域/人群限制调整）
	"WebcastVisibilityRangeChangeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_VisibilityRangeChangeMessage{}
	},

	// 反馈卡片消息（弹出用户反馈收集窗口）
	"WebcastFeedbackCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_FeedbackCardMessage{}
	},

	// 印章消息（用户等级/身份徽章展示）
	"WebcastStampMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_StampMessage{}
	},

	// 自定义卡片消息（主播自定义图文信息卡）
	"WebcastCustomizedCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_CustomizedCardMessage{}
	},

	// 观众入场消息（新观众进入直播间提示）
	"WebcastAudienceEntranceMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_AudienceEntranceMessage{}
	},
	// 直播间信息变更消息（房间名称/封面等更新）
	"WebcastGroupLiveContainerChangeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GroupLiveContainerChangeMessage{}
	},
}
