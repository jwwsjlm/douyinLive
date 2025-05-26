package generated

import (
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var messageMap = map[string]func() protoreflect.ProtoMessage{
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
var newMessage = map[string]func() protoreflect.ProtoMessage{
	//来源:https://github.com/qiaoruntao/douyin_contract/blob/master/mapping.json
	//https://github.com/Remember-the-past/douyin_proto/blob/main/method%E5%AF%B9%E5%BA%94proto%E5%85%B3%E7%B3%BB.md
	// 直播文字聊天消息（普通文本弹幕）
	// Live text chat message (ordinary text danmaku)
	"WebcastChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ChatMessage{} },

	// 直播礼物赠送消息（用户送礼行为通知）
	// Live gift presentation message (notification of user gifting behavior)
	"WebcastGiftMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GiftMessage{} },

	// 直播点赞消息（用户点击直播间点赞按钮）
	// Live like message (user clicks the like button in the live broadcast room)
	"WebcastLikeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LikeMessage{} },

	// 直播间成员变动消息（加入/离开/关注等）
	// Live broadcast room member change message (join/leave/follow, etc.)
	"WebcastMemberMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_MemberMessage{} },

	// 直播社交互动消息（分享/关注/邀请等社交行为）
	// Live social interaction message (social behaviors such as sharing/following/inviting)
	"WebcastSocialMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SocialMessage{} },

	// 直播间用户序列消息（维护在线用户列表顺序）
	// Live broadcast room user sequence message (maintains the order of the online user list)
	"WebcastRoomUserSeqMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomUserSeqMessage{} },

	// 粉丝团相关消息（入团、升级、粉丝任务通知）
	// Fans club related messages (joining the club, upgrading, fan task notifications)
	"WebcastFansclubMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_FansclubMessage{} },

	// 直播控制消息（禁言、清屏、设置管理员等操作）
	// Live broadcast control message (operations such as muting, clearing the screen, setting administrators)
	"WebcastControlMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ControlMessage{} },

	// 表情聊天消息（带Emoji表情的弹幕）
	// Emoji chat message (danmaku with Emoji expressions)
	"WebcastEmojiChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_EmojiChatMessage{} },

	// 直播间统计消息（在线人数、互动数据、礼物收益）
	// Live broadcast room statistics message (number of online viewers, interaction data, gift revenue)
	"WebcastRoomStatsMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStatsMessage{} },

	// 直播间通用通知消息（系统公告、活动提示）
	// Live broadcast room general notification message (system announcements, activity prompts)
	"WebcastRoomMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomMessage{} },

	// 小时榜入口消息（触发 hourly ranking list 显示）
	// Hourly ranking list entrance message (triggers the display of the hourly ranking list)
	"WebcastRanklistHourEntranceMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RanklistHourEntranceMessage{} },

	// 直播间排名消息（礼物贡献榜、互动榜实时更新）
	// Live broadcast room ranking message (real-time updates of the gift contribution list and interaction list)
	"WebcastRoomRankMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomRankMessage{} },

	// 直播间内横幅消息（顶部滚动活动公告）
	// In-room banner message (scrolling activity announcements at the top)
	"WebcastInRoomBannerMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_InRoomBannerMessage{} },

	// 直播间数据同步消息（多端状态一致性更新）
	// Live broadcast room data synchronization message (consistent status updates across multiple devices)
	"WebcastRoomDataSyncMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomDataSyncMessage{} },

	// 盲盒临时状态消息（抽奖活动进度通知）
	// Lucky box temporary status message (notification of the progress of the lottery activity)
	"WebcastLuckyBoxTempStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxTempStatusMessage{} },

	// 直播间装饰修改消息（背景、边框等视觉元素调整）
	// Live broadcast room decoration modification message (adjustment of visual elements such as background and border)
	"WebcastDecorationModifyMethod": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_DecorationModifyMessage{} },

	// 连麦观众K歌消息（多人连麦K歌场景）
	// Linked microphone audience KTV message (multi-person linked microphone KTV scenario)
	"WebcastLinkMicAudienceKtvMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LinkMicAudienceKtvMessage{} },

	// 直播流自适应消息（动态调整视频清晰度）
	// Live stream adaptation message (dynamically adjusts video clarity)
	"WebcastRoomStreamAdaptationMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomStreamAdaptationMessage{} },

	// 问答观众状态消息（互动答题活动参与情况）
	// Quiz audience status message (participation status in interactive quiz activities)
	"WebcastQuizAudienceStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_QuizAudienceStatusMessage{} },

	// 热门聊天消息（高互动量弹幕聚合展示）
	// Hot chat message (aggregated display of high-interaction danmakus)
	"WebcastHotChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotChatMessage{} },

	// 热门直播间推荐消息（推送同类热门房间）
	// Hot live broadcast room recommendation message (pushes similar popular rooms)
	"WebcastHotRoomMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HotRoomMessage{} },

	// 语音聊天消息（纯音频直播互动）
	// Voice chat message (pure audio live broadcast interaction)
	"WebcastAudioChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_AudioChatMessage{} },

	// 直播间媒体通知消息（重要事件弹窗提示）
	// Live broadcast room media notification message (pop-up prompt for important events)
	"WebcastRoomNotifyMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NotifyMessage{} },

	// 盲盒活动消息（抽奖结果、奖励发放通知）
	// Lucky box activity message (lottery results, reward distribution notifications)
	"WebcastLuckyBoxMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxMessage{} },

	// 粉丝票更新消息（粉丝团专属积分变动）
	// Fan ticket update message (changes in fan club exclusive points)
	"WebcastUpdateFanTicketMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_UpdateFanTicketMessage{} },

	// 屏幕弹幕消息（全屏滚动特效弹幕）
	// Screen danmaku message (full-screen scrolling special effect danmaku)
	"WebcastScreenChatMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ScreenChatMessage{} },

	// 通知特效消息（礼物连击、关注等动画效果）
	// Notification effect message (animation effects such as gift combos and follows)
	"WebcastNotifyEffectMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NotifyEffectMessage{} },

	// 绑定礼物消息（任务奖励礼物发放）
	// Binding gift message (distribution of task reward gifts)
	"WebcastBindingGiftMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_BindingGiftMessage{} },

	// 临时状态区域触发消息（如福袋领取区域进入通知）
	// Temporary state area trigger message (such as notification of entering the lucky bag collection area)
	"WebcastTempStateAreaReachMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_TempStateAreaReachMessage{} },

	// 成长任务消息（观看时长、互动次数奖励通知）
	// Growth task message (notification of rewards for watch time and interaction次数)
	"WebcastGrowthTaskMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GrowthTaskMessage{} },

	// 第三方游戏基础消息（直播内嵌游戏互动）
	// Third-party game base message (interactive games embedded in the live broadcast)
	"WebcastGameCPBaseMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameCPBaseMessage{} },

	// 夹层边框特效消息（直播间主题装饰边框）
	// Sandwich border effect message (theme decorative border of the live broadcast room)
	"WebcastSandwichBorderMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SandwichBorderMessage{} },

	// 直播电商通用消息（商品上架、促销活动通知）
	// Live e-commerce general message (product上架, promotional activity notifications)
	"WebcastLiveEcomGeneralMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveEcomGeneralMessage{}
	},

	// 直播购物消息（下单、抢购、优惠券领取通知）
	// Live shopping message (order placement, flash sale, coupon collection notifications)
	"WebcastLiveShoppingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LiveShoppingMessage{}
	},

	// 聊天内容点赞消息（对特定弹幕的点赞互动）
	// Chat content like message (like interaction for specific danmakus)
	"WebcastChatLikeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ChatLikeMessage{}
	},

	// 连麦点歌评分消息（K歌直播中的演唱评分）
	// Linked microphone song order scoring message (singing score in KTV live broadcast)
	"WebcastLinkmicOrderSingScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingScoreMessage{}
	},

	// 连麦贡献消息（连麦嘉宾的礼物贡献统计）
	// Linked microphone contribution message (statistics of gift contributions from linked microphone guests)
	"WebcastLinkerContributeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkerContributeMessage{}
	},

	// 连麦发送表情消息（连麦场景下的表情互动）
	// Linked microphone send emoji message (emoji interaction in the linked microphone scenario)
	"WebcastLinkMicSendEmojiMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicSendEmojiMessage{}
	},

	// 连麦播放模式消息（伴奏/原唱模式切换通知）
	// Linked microphone play mode message (notification of accompaniment/original mode switching)
	"WebcastLinkmicPlaymodeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlaymodeMessage{}
	},

	// 活动表情组消息（限时活动专属表情包）
	// Event emoji group message (exclusive emoji packs for time-limited events)
	"WebcastActivityEmojiGroupsMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ActivityEmojiGroupsMessage{}
	},

	// 连麦模式更新分数消息（实时K歌评分更新）
	// Linked microphone mode update score message (real-time KTV score update)
	"WebcastLinkmicPlayModeUpdateScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicPlayModeUpdateScoreMessage{}
	},

	// 连麦操作消息（申请/接受/挂断连麦请求）
	// Linked microphone operation message (apply/accept/hang up linked microphone request)
	"WebcastLinkMicMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicMethod{}
	},

	// 分享链接消息（直播间分享行为通知）
	// Share link message (notification of live broadcast room sharing behavior)
	"WebcastLinkMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMessage{}
	},

	// 连麦点歌消息（观众请求演唱歌曲）
	// Linked microphone song order message (audience requests to sing a song)
	"WebcastLinkmicOrderSingMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicOrderSingMessage{}
	},

	// 连麦位置调整消息（主麦/副麦布局变更）
	// Linked microphone position adjustment message (change in the layout of the main and auxiliary microphones)
	"WebcastLinkMicPositionMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicPositionMessage{}
	},

	// 连麦嘉宾放大消息（特写展示某连麦者画面）
	// Linked microphone guest enlargement message (close-up display of a linked microphone participant's画面)
	"WebcastLinkmicEnlargeGuestMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicEnlargeGuestMessage{}
	},

	// 连麦收益消息（嘉宾礼物分成比例通知）
	// Linked microphone revenue message (notification of the guest's gift sharing ratio)
	"WebcastLinkmicProfitMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkmicProfitMessage{}
	},

	// 礼物图标闪烁特效消息（高价值礼物全屏动画）
	// Gift icon flash effect message (full-screen animation for high-value gifts)
	"WebcastGiftIconFlashMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftIconFlashMessage{}
	},

	// 礼物排序消息（调整礼物列表展示优先级）
	// Gift sorting message (adjusts the display priority of the gift list)
	"WebcastGiftSortMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GiftSortMessage{}
	},

	// 特权弹幕消息（会员专属特效弹幕）
	// Privilege danmaku message (member-exclusive special effect danmaku)
	"WebcastPrivilegeScreenChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_PrivilegeScreenChatMessage{}
	},

	// 展示聊天消息（精选弹幕置顶显示）
	// Exhibition chat message (selected danmakus displayed at the top)
	"WebcastExhibitionChatMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ExhibitionChatMessage{}
	},

	// 货架交易数据消息（商品实时销量/库存更新）
	// Shelf transaction data message (real-time product sales/inventory updates)
	"WebcastShelfTradeDataMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ShelfTradeDataMessage{}
	},

	// 可见范围变更消息（直播内容地域/人群限制调整）
	// Visibility range change message (adjustment of regional/audience restrictions for live content)
	"WebcastVisibilityRangeChangeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_VisibilityRangeChangeMessage{}
	},

	// 反馈卡片消息（弹出用户反馈收集窗口）
	// Feedback card message (pops up a user feedback collection window)
	"WebcastFeedbackCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_FeedbackCardMessage{}
	},

	// 印章消息（用户等级/身份徽章展示）
	// Stamp message (display of user level/identity badges)
	"WebcastStampMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_StampMessage{}
	},

	// 自定义卡片消息（主播自定义图文信息卡）
	// Customized card message (anchor-customized graphic information card)
	"WebcastCustomizedCardMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_CustomizedCardMessage{}
	},

	// 观众入场消息（新观众进入直播间提示）
	// Audience entrance message (prompt when a new viewer enters the live broadcast room)
	"WebcastAudienceEntranceMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_AudienceEntranceMessage{}
	},
	// 直播间信息变更消息（房间名称/封面等更新）
	// Live broadcast room information change message (updates to room name/cover, etc.)
	"WebcastGroupLiveContainerChangeMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_GroupLiveContainerChangeMessage{}
	},
	// 直播间信息变更消息（房间名称/封面等更新）
	// Live broadcast room information change message (updates to room name/cover, etc.)
	"WebcastBackupSEIMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BackupSEIMessage{}
	},
	// 直播间信息变更消息（房间名称/封面等更新）
	// Live broadcast room information change message (updates to room name/cover, etc.)
	"WebcastKtvMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_KtvMessage{}
	},
	// 直播数据生命周期消息（直播开始/结束/中断等状态变更）
	// Live broadcast data lifecycle message (status changes such as live broadcast start/end/interruption)
	"WebcastDataLifeLiveMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_DataLifeLiveMessage{}
	},
	// 新抽奖活动消息（福袋/红包等活动通知）
	// New lottery event message (notifications for lucky bag/red envelope activities)
	"WebcastLotteryEventNewMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LotteryEventNewMessage{}
	},
	// 连麦操作消息（申请/接受/拒绝/挂断连麦等）
	// Linked microphone operation message (apply/accept/reject/hang up linked microphone, etc.)
	"LinkMicMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicMethod{}
	},
	// 收益互动评分消息（直播收益与互动效果评分）
	// Profit interaction score message (evaluation of live broadcast revenue and interaction effects)
	"WebcastProfitInteractionScoreMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ProfitInteractionScoreMessage{}
	},
	// 通用吐司消息（轻量级通知提示）
	// Common toast message (lightweight notification prompt)
	"WebcastCommonToastMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_CommonToastMessage{}
	},
	// 连麦对战结束消息（连麦PK结束结果通知）
	// Linked microphone battle finish message (notification of the result of a linked microphone PK)
	"WebcastLinkMicBattleFinishMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicBattleFinish{}
	},
	// 直播间装饰更新消息（主题/背景/特效等装饰更新）
	// Live broadcast room decoration update message (updates to decorations such as themes/backgrounds/special effects)
	"WebcastDecorationUpdateMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_DecorationUpdateMessage{}
	},
	// 直播收益游戏状态消息（收益互动游戏状态更新，如答题、竞猜等）
	"WebcastProfitGameStatusMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_ProfitGameStatusMessage{}
	},
	// 连麦对战操作消息（连麦PK挑战、接受、开始等操作）
	// Linked microphone battle operation message (operations such as challenging, accepting, and starting a linked microphone PK)
	"WebcastLinkMicBattleMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicBattle{}
	},

	// 对战团队任务消息（连麦PK团队任务进度和奖励通知）
	// Battle team task message (notification of progress and rewards for linked microphone PK team tasks)
	"WebcastBattleTeamTaskMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BattleTeamTaskMessage{}
	},
	// 轻量级礼物消息（简易动画礼物通知，如点赞动画礼物）
	// Lightweight gift message (notification for simple animated gifts, such as like animation gifts)
	"WebcastLightGiftMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LightGiftMessage{}
	},
	// 连麦军团操作消息（连麦场景下军团相关操作，如军团对抗、协作等）
	"WebcastLinkMicArmiesMethod": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkMicArmies{}
	},
	// 连麦军团状态消息（连麦场景下军团状态更新，如军团成员变动、军团任务进度等）
	"WebcastBattleSeasonPKResultMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BattleSeasonPKResultMessage{}
	},
	// 连麦军团状态消息（连麦场景下军团状态更新，如军团成员变动、军团任务进度等）
	"WebcastBattlePowerContainerMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BattlePowerContainerMessage{}
	},
	// 对战结束惩罚消息（连麦PK结束后惩罚结果通知）
	// Battle end punishment message (notification of punishment result after linked streaming PK)
	"WebcastBattleEndPunishMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BattleEndPunishMessage{}
	},

	// 对战特效容器消息（连麦PK特效集合状态更新）
	// Battle effect container message (status update of linked streaming PK effect collection)
	"WebcastBattleEffectContainerMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_BattleEffectContainerMessage{}
	},

	// 置顶特效消息（直播间顶部特效展示通知）
	// Top effect message (notification of top visual effects display in live room)
	"WebcastTopEffectMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_TopEffectMessage{}
	},

	// 连麦设置通知消息（连麦功能参数调整通知）
	// Link setting notification message (linked streaming configuration changes notification)
	"WebcastLinkSettingNotifyMessage": func() protoreflect.ProtoMessage {
		return &new_douyin.Webcast_Im_LinkSettingNotifyMessage{}
	},
	// 多人匹配对战消息（多人游戏匹配状态通知）
	// Multi-player match battle message (multiplayer game matching status notification)
	"WebcastBattleMultiMatchMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_BattleMultiMatchMessage{} },

	// KTV播放模式开始消息（KTV模式启动通知）
	// KTV play mode start message (notification of KTV mode activation)
	"WebcastKTVPlayModeStartMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_KTVPlayModeStartMessage{} },

	// KTV用户演唱热度消息（用户演唱实时热度值）
	// KTV user singing heat message (real-time singing heat value of users)
	"WebcastKTVUserSingingHotMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_KTVUserSingingHotMessage{} },

	// KTV歌手热度排名消息（歌手实时排名位置）
	// KTV singer heat ranking message (real-time singer ranking position)
	"WebcastKTVSingerHotRankPosMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_KTVSingerHotRankPosMessage{} },

	// 房间权限消息（直播间权限变更通知）
	// Room authority message (live room permission change notification)
	"WebcastRoomAuthMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomAuthMessage{} },

	// 传送门消息（特殊效果传送门触发通知）
	// Portal message (special effect portal trigger notification)
	"WebcastPortalMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_PortalMessage{} },

	// 消息删除通知（聊天消息删除操作通知）
	// Message deletion notification (chat message delete operation notification)
	"WebcastImDeleteMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ImDeleteMessage{} },

	// 用户特权变更消息（用户权限等级变化通知）
	// User privilege change message (user privilege level change notification)
	"WebcastUserPrivilegeChangeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_UserPrivilegeChangeMessage{} },

	// 房间验证消息（直播间安全验证通知）
	// Room verification message (live room security verification notification)
	"WebcastRoomVerifyMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomVerifyMessage{} },

	// 资产变动消息（用户虚拟资产变动通知）
	// Asset change message (user virtual asset change notification)
	"WebcastAssetMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_AssetMessage{} },

	// 流同步消息（多平台直播流状态同步）
	// Stream synchronization message (multi-platform live stream status synchronization)
	"WebcastSyncStreamMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SyncStreamMessage{} },

	// 游戏CP主播推广消息（游戏合作主播推广信息）
	// Game CP anchor promotion message (game collaboration anchor promotion information)
	"WebcastGameCPAnchorPromoteInfoMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameCPAnchorPromoteInfoMessage{} },

	// 弹窗通知消息（系统级弹窗提示信息）
	// Toast notification message (system-level popup notification)
	"WebcastToastMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ToastMessage{} },

	// 礼物投票消息（礼物投票活动通知）
	// Gift vote message (gift voting activity notification)
	"WebcastGiftVoteMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GiftVoteMessage{} },

	// 连麦团队战消息（多人连麦对战状态通知）
	// Linkmic team battle message (multi-user linked streaming battle status notification)
	"WebcastLinkmicTeamfightMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LinkmicTeamfightMessage{} },

	// 商品变更消息（直播带货商品状态变更）
	// Product change message (live commerce product status change)
	"WebcastProductChangeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_ProductChangeMessage{} },

	// 双赞榜首消息（双赞活动榜首用户通知）
	// Double-like top user message (top user notification for double-like activity)
	"WebcastDoubleLikeTopUserMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_DoubleLikeTopUserMessage{} },

	// 阳光日榜消息（地区阳光日榜排名通知）
	// Sun daily rank message (regional sun daily ranking notification)
	"WebcastSunDailyRankMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SunDailyRankMessage{} },

	// 经纪人通知消息（直播经纪人系统通知）
	// Broker notification message (live streaming broker system notification)
	"WebcastBrokerNotifyMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_BrokerNotifyMessage{} },

	// 电商消息（直播电商相关操作通知）
	// Commerce message (live e-commerce related operation notification)
	"WebcastCommerceMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_CommerceMessage{} },

	// 角落达成消息（特定区域互动目标达成）
	// Corner reach message (specific area interaction goal achievement)
	"WebcastCornerReachMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_CornerReachMessage{} },

	// 游戏观众入口消息（游戏观战模式入口通知）
	// Game audience entrance message (game spectator mode entrance notification)
	"WebcastGameAncAudEntranceMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameAncAudEntranceMessage{} },

	// 游戏CP用户下载消息（游戏合作用户下载通知）
	// Game CP user download message (game collaboration user download notification)
	"WebcastGameCPUserDownloadMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameCPUserDownloadMessage{} },

	// 游戏状态消息（直播间游戏状态更新）
	// Game status message (live room game status update)
	"WebcastGameStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_GameStatusMessage{} },

	// 高亮评论消息（精选评论突出显示通知）
	// Highlight comment message (featured comment highlighting notification)
	"WebcastHighlightComment": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_HighlightComment{} },

	// 横幅刷新消息（直播间横幅内容刷新）
	// Banner refresh message (live room banner content refresh)
	"WebcastInRoomBannerRefreshMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_InRoomBannerRefreshMessage{} },

	// 互动应用状态消息（第三方互动应用状态通知）
	// Interactive app status message (third-party interactive app status notification)
	"WebcastInteractOpenAppStatusMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_InteractOpenAppStatusMessage{} },

	// 直播电商消息（电商相关功能通知）
	// Live e-commerce message (e-commerce related feature notification)
	"WebcastLiveEcomMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LiveEcomMessage{} },

	// 抽奖结果消息（抽奖活动中奖结果通知）
	// Lottery result message (lottery activity result notification)
	"WebcastLotteryDrawResultEventMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LotteryDrawResultEventMessage{} },

	// 幸运盒结束消息（幸运盒活动结束通知）
	// Lucky box end message (lucky box activity end notification)
	"WebcastLuckyBoxEndMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LuckyBoxEndMessage{} },

	// 系统通知消息（平台级系统通知）
	// System notice message (platform-level system notification)
	"WebcastNoticeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_NoticeMessage{} },

	// 奖品通知消息（用户获奖通知）
	// Prize notice message (user prize notification)
	"WebcastPrizeNoticeMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_PrizeNoticeMessage{} },

	// 拉流更新消息（视频流参数更新通知）
	// Pull stream update message (video stream parameters update notification)
	"WebcastPullStreamUpdateMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_PullStreamUpdateMessage{} },

	// 房间简介消息（直播间简介信息更新）
	// Room introduction message (live room introduction update)
	"WebcastRoomIntroMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_RoomIntroMessage{} },

	// 音频背景图消息（语音直播背景图更新）
	// Audio background image message (audio live background image update)
	"WebcastAudioBGImgMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_AudioBGImgMessage{} },

	// 连麦引导消息（连麦功能使用引导提示）
	// Linkmic guide message (linked streaming feature tutorial)
	"WebcastLinkMicGuideMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_LinkMicGuideMessage{} },

	// 阳光日榜地区排名消息（地区阳光日榜排名）
	// Sun daily region rank message (regional sun daily ranking)
	"WebcastSunDailyRegionRankMessage": func() protoreflect.ProtoMessage { return &new_douyin.Webcast_Im_SunDailyRankMessage{} },
}
