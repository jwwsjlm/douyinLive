package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
)

// appendJSONStringField 向已有 JSON 对象字节流追加一个字符串字段。
// appendJSONStringField appends one string field to an existing JSON object buffer.
// 参数/Parameters:
//   - dst: 已有 JSON 对象字节流。 Existing JSON object bytes.
//   - key: 要追加的字段名。 Field name to append.
//   - value: 要追加的字段值。 Field value to append.
func appendJSONStringField(dst []byte, key, value string) []byte {
	if len(dst) == 0 || dst[len(dst)-1] != '{' {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendQuote(dst, value)
	return dst
}

// buildEventJSON 将解析后的 protobuf JSON 补充直播间元数据。
// buildEventJSON enriches parsed protobuf JSON with live room metadata.
// 参数/Parameters:
//   - jsonBytes: protobuf 转出的 JSON 字节。 JSON bytes produced from protobuf.
//   - method: 抖音消息方法名。 Douyin message method name.
//   - liveName: 主播昵称。 Live owner nickname.
//   - title: 直播间标题。 Live room title.
//   - avatarThumb: 主播头像缩略图地址。 Live owner avatar thumbnail URL.
func (r *Room) buildEventJSON(jsonBytes []byte, method, liveName, title, avatarThumb string) ([]byte, error) {
	jsonBytes = bytes.TrimSpace(jsonBytes)
	if len(jsonBytes) < 2 || jsonBytes[0] != '{' || jsonBytes[len(jsonBytes)-1] != '}' || !json.Valid(jsonBytes) {
		return nil, fmt.Errorf("无效的事件 JSON")
	}

	extra := 64 + len(method) + len(liveName) + len(title) + len(avatarThumb)
	result := make([]byte, 0, len(jsonBytes)+extra)
	result = append(result, '{')
	objectBody := bytes.TrimSpace(jsonBytes[1 : len(jsonBytes)-1])
	if len(objectBody) > 0 {
		result = append(result, objectBody...)
	}
	result = appendJSONStringField(result, "method", method)
	result = appendJSONStringField(result, "livename", liveName)
	result = appendJSONStringField(result, "title", title)
	result = appendJSONStringField(result, "avatarThumb", avatarThumb)

	result = append(result, '}')
	return result, nil
}

func (r *Room) updateMetadataFromDouyinLive(d *douyinLive.DouyinLive) {
	if d == nil {
		return
	}
	liveName := d.GetName()
	title := d.GetTitle()
	avatarThumb := d.GetAvatarThumb()
	accountOnly := d.HasAnchorOnlyPageIdentity()

	r.mu.Lock()
	if liveName != "" {
		r.liveName = liveName
	}
	if title != "" {
		r.title = title
	}
	if avatarThumb != "" {
		r.avatarThumb = avatarThumb
	}
	if accountOnly {
		r.accountOnly = true
	} else {
		r.accountOnly = false
	}
	r.mu.Unlock()
}

func (r *Room) metadataSnapshot() (string, string, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.liveName, r.title, r.avatarThumb, r.accountOnly
}

type systemStatusMessage struct {
	Type                 string `json:"type"`
	Event                string `json:"event"`
	Code                 string `json:"code"`
	Valid                *bool  `json:"valid"`
	Live                 *bool  `json:"live"`
	Status               string `json:"status"`
	StatusText           string `json:"status_text"`
	RoomID               string `json:"room_id"`
	LiveName             string `json:"live_name"`
	Title                string `json:"title"`
	AvatarThumb          string `json:"avatar_thumb"`
	HasRoom              *bool  `json:"has_room"`
	AccountOnly          *bool  `json:"account_only"`
	Message              string `json:"message"`
	Suggestion           string `json:"suggestion"`
	RetryIntervalSeconds *int   `json:"retry_interval_seconds,omitempty"`
	Ended                *bool  `json:"ended,omitempty"`
}

func marshalSystemStatusMessage(message systemStatusMessage) []byte {
	payload, err := json.Marshal(message)
	if err != nil {
		return []byte(`{"type":"system","event":"live_status","code":"SERIALIZATION_ERROR"}`)
	}
	return payload
}

func boolPtr(value bool) *bool { return &value }

// offlineStatusMessage 构造未开播状态通知。
// offlineStatusMessage builds the offline status notification.
func (r *Room) offlineStatusMessage() []byte {
	liveName, title, avatarThumb, accountOnly := r.metadataSnapshot()
	falseValue := false
	hasRoom := !accountOnly
	message := "直播间当前未开播，服务端会保持连接并继续轮询"
	code, status, statusText := "ROOM_OFFLINE", "offline", "直播间未开播"
	suggestion := "客户端不需要重连，保持当前 WebSocket 连接等待开播通知"
	if accountOnly {
		code, status, statusText = "ACCOUNT_OFFLINE_NO_ROOM", "account_offline", "账号存在但当前没有直播间"
		message = "账号存在，但网页没有返回直播间房间对象，可能是该账号从未开播或当前未创建直播间，当前按未开播处理"
		suggestion = "客户端不需要重连，保持当前 WebSocket 连接；如果该账号后续开播，服务端会自动切换为直播连接"
	}
	retrySeconds := int(r.notifyInterval / time.Second)
	return marshalSystemStatusMessage(systemStatusMessage{Type: "system", Event: "live_status", Code: code, Valid: boolPtr(true), Live: &falseValue, Status: status, StatusText: statusText, RoomID: r.id, LiveName: liveName, Title: title, AvatarThumb: avatarThumb, HasRoom: &hasRoom, AccountOnly: &accountOnly, Message: message, Suggestion: suggestion, RetryIntervalSeconds: &retrySeconds})
}

// statusUnknownMessage 构造暂时无法确认状态的通知，避免把风控验证页误报为房间不存在。
// statusUnknownMessage builds an indeterminate status notification instead of misclassifying a challenge page as a missing room.
func (r *Room) statusUnknownMessage() []byte {
	liveName, title, avatarThumb, accountOnly := r.metadataSnapshot()
	r.mu.Lock()
	knownValid := r.knownValid
	r.mu.Unlock()
	var hasRoom, accountOnlyValue *bool
	switch {
	case accountOnly:
		falseValue, trueValue := false, true
		hasRoom, accountOnlyValue = &falseValue, &trueValue
	case knownValid:
		trueValue, falseValue := true, false
		hasRoom, accountOnlyValue = &trueValue, &falseValue
	}
	return marshalSystemStatusMessage(systemStatusMessage{Type: "system", Event: "live_status", Code: "ROOM_STATUS_UNKNOWN", Valid: boolPtr(false), Live: nil, Status: "unknown", StatusText: "暂时无法确认直播状态", RoomID: r.id, LiveName: liveName, Title: title, AvatarThumb: avatarThumb, HasRoom: hasRoom, AccountOnly: accountOnlyValue, Message: "上游页面或接口暂时未返回可验证的房间状态，服务端会继续轮询", Suggestion: "客户端保持当前 WebSocket 连接，不要立即重连", RetryIntervalSeconds: nil})
}

// offlineEndedStatusMessage 构造已下播状态通知。
// offlineEndedStatusMessage builds the ended-offline status notification.
func (r *Room) offlineEndedStatusMessage() []byte {
	liveName, title, avatarThumb, _ := r.metadataSnapshot()
	falseValue := false
	ended := true
	retrySeconds := int(r.notifyInterval / time.Second)
	return marshalSystemStatusMessage(systemStatusMessage{Type: "system", Event: "live_status", Code: "ROOM_ENDED", Valid: boolPtr(true), Live: &falseValue, Status: "ended", StatusText: "直播间已下播", RoomID: r.id, LiveName: liveName, Title: title, AvatarThumb: avatarThumb, Message: "直播间已经下播，服务端会保持连接并等待再次开播", Suggestion: "客户端不需要重连，保持当前 WebSocket 连接等待下一次开播", Ended: &ended, RetryIntervalSeconds: &retrySeconds})
}

// onlineStatusMessage 构造已开播状态通知。
// onlineStatusMessage builds the online status notification.
func (r *Room) onlineStatusMessage() []byte {
	liveName, title, avatarThumb, _ := r.metadataSnapshot()
	trueValue := true
	return marshalSystemStatusMessage(systemStatusMessage{Type: "system", Event: "live_status", Code: "ROOM_ONLINE", Valid: boolPtr(true), Live: &trueValue, Status: "online", StatusText: "直播间已开播", RoomID: r.id, LiveName: liveName, Title: title, AvatarThumb: avatarThumb, Message: "直播间已开播，后续将开始推送弹幕、礼物、点赞等直播消息", Suggestion: "客户端可以开始正常处理直播消息", RetryIntervalSeconds: nil})
}

// notifyOfflineStatus 广播未开播状态通知。
// notifyOfflineStatus broadcasts the offline status notification.
func (r *Room) notifyOfflineStatus() {
	r.Broadcast(r.offlineStatusMessage())
}

// notifyStatusUnknown 广播暂时无法确认状态的通知。
// notifyStatusUnknown broadcasts an indeterminate live-status notification.
func (r *Room) notifyStatusUnknown() {
	r.Broadcast(r.statusUnknownMessage())
}

// notifyMonitorStatus 按当前监控状态广播未知或未开播通知，避免把风控页误报为未开播。
// notifyMonitorStatus broadcasts the current indeterminate/offline monitor state without misclassifying challenge pages.
func (r *Room) notifyMonitorStatus() {
	if r.isStatusUnknown() {
		r.notifyStatusUnknown()
		return
	}
	r.notifyOfflineStatus()
}

// notifyOfflineEndedStatus 广播已下播状态通知。
// notifyOfflineEndedStatus broadcasts the ended-offline status notification.
func (r *Room) notifyOfflineEndedStatus() {
	r.Broadcast(r.offlineEndedStatusMessage())
}

// notifyOnlineStatus 广播已开播状态通知。
// notifyOnlineStatus broadcasts the online status notification.
func (r *Room) notifyOnlineStatus() {
	r.Broadcast(r.onlineStatusMessage())
}
