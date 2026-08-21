package douyinLive

import (
	"context"
	"errors"
	"fmt"
)

// LiveStatusCode identifies the result of a live-room status check.
// LiveStatusCode 表示直播间状态检查结果。
type LiveStatusCode string

const (
	// LiveStatusOnline means the room is currently live.
	// LiveStatusOnline 表示直播间正在直播。
	LiveStatusOnline LiveStatusCode = "online"
	// LiveStatusOffline means the account has a room, but it is not live now.
	// LiveStatusOffline 表示账号存在房间，但当前未开播。
	LiveStatusOffline LiveStatusCode = "offline"
	// LiveStatusNoRoom means the account exists but has no current room.
	// LiveStatusNoRoom 表示账号存在，但当前没有直播间。
	LiveStatusNoRoom LiveStatusCode = "account_no_room"
	// LiveStatusUnknown means the upstream response could not be verified.
	// LiveStatusUnknown 表示上游响应暂时无法验证直播状态。
	LiveStatusUnknown LiveStatusCode = "unknown"
	// LiveStatusNotFound means the requested room identifier was definitively not found.
	// LiveStatusNotFound 表示请求的直播间标识已被明确判定不存在。
	LiveStatusNotFound LiveStatusCode = "not_found"
)

// LiveStatus is the structured result returned by CheckLiveStatus.
// LiveStatus 是 CheckLiveStatus 返回的结构化状态结果。
type LiveStatus struct {
	// Code is the normalized status code.
	// Code 是标准化状态码。
	Code LiveStatusCode `json:"code"`
	// Live is non-nil only when the live state was verified.
	// Live 仅在直播状态已确认时非 nil。
	Live *bool `json:"live,omitempty"`
	// HasRoom reports whether a room identity was verified.
	// HasRoom 表示是否确认存在房间身份。
	HasRoom *bool `json:"has_room,omitempty"`
	// AccountOnly reports an account identity without a current room.
	// AccountOnly 表示只确认了账号身份，没有当前房间。
	AccountOnly *bool `json:"account_only,omitempty"`
	// LiveID is the identifier supplied to the constructor.
	// LiveID 是构造实例时传入的直播间标识。
	LiveID string `json:"live_id"`
	// RoomID is the verified long room ID when available.
	// RoomID 是可用时解析到的真实长房间 ID。
	RoomID string `json:"room_id,omitempty"`
	// UserUniqueID is the anchor/user identifier used by Douyin's live APIs.
	// UserUniqueID 是抖音直播接口使用的主播/用户唯一标识。
	UserUniqueID string `json:"user_unique_id,omitempty"`
	// LiveName is the anchor display name when available.
	// LiveName 是可用时解析到的主播名称。
	LiveName string `json:"live_name,omitempty"`
	// Title is the room title when available.
	// Title 是可用时解析到的直播标题。
	Title string `json:"title,omitempty"`
	// AvatarThumb is the anchor avatar thumbnail URL when available.
	// AvatarThumb 是可用时解析到的主播头像缩略图地址。
	AvatarThumb string `json:"avatar_thumb,omitempty"`
}

// CheckLiveStatus checks the current room status without opening a WebSocket.
// CheckLiveStatus 检查当前直播状态，但不会建立上游 WebSocket 连接。
//
// A definitive room-not-found result returns LiveStatusNotFound together with
// ErrRoomNotFound. Network, challenge-page, timeout, and parsing failures return
// LiveStatusUnknown together with an error so callers can distinguish an
// unverified result from a confirmed offline room.
// 网络、风控验证页、超时和解析失败会返回 LiveStatusUnknown 以及 error，
// 调用方可以区分“无法确认”和“已确认未开播”。
func (dl *DouyinLive) CheckLiveStatus(ctx context.Context) (LiveStatus, error) {
	if err := dl.ensureUsable(); err != nil {
		return LiveStatus{Code: LiveStatusUnknown}, fmt.Errorf("%w: %w", ErrLiveStatusUnknown, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := dl.requestContextWithParent(ctx)
	defer cancel()

	if err := requestCtx.Err(); err != nil {
		return dl.liveStatusResult(LiveStatusUnknown), fmt.Errorf("%w: live_id=%s: %w", ErrLiveStatusUnknown, dl.liveID, err)
	}

	isLive, err := dl.fetchLiveStatusFromAPIWithContext(requestCtx)
	if err != nil {
		switch {
		case errors.Is(err, ErrRoomNotFound):
			status := dl.liveStatusResult(LiveStatusNotFound)
			return status, err
		case errors.Is(err, ErrLiveStatusUnknown), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			status := dl.liveStatusResult(LiveStatusUnknown)
			if errors.Is(err, ErrLiveStatusUnknown) {
				return status, err
			}
			return status, fmt.Errorf("%w: live_id=%s: %w", ErrLiveStatusUnknown, dl.liveID, err)
		default:
			status := dl.liveStatusResult(LiveStatusUnknown)
			return status, fmt.Errorf("%w: live_id=%s: %w", ErrLiveStatusUnknown, dl.liveID, err)
		}
	}

	dl.setLiveStatus(isLive)
	status := dl.liveStatusResult("")
	if status.Code == LiveStatusUnknown {
		return status, fmt.Errorf("%w: live_id=%s", ErrLiveStatusUnknown, dl.liveID)
	}
	return status, nil
}

func statusCodeForSnapshot(info roomInfoSnapshot, isLive, known bool) LiveStatusCode {
	if !known {
		return LiveStatusUnknown
	}
	if isLive {
		return LiveStatusOnline
	}
	if info.anchorOnly {
		return LiveStatusNoRoom
	}
	if info.roomID == "" && info.liveName == "" && info.title == "" && info.avatarThumb == "" {
		return LiveStatusUnknown
	}
	return LiveStatusOffline
}

func boolPointer(value bool) *bool {
	return &value
}

func (dl *DouyinLive) liveStatusResult(code LiveStatusCode) LiveStatus {
	dl.mu.Lock()
	isLive := dl.isLive
	known := dl.liveStatusKnown
	info := roomInfoSnapshot{
		liveID:      dl.liveID,
		roomID:      dl.roomID,
		pushID:      dl.pushID,
		liveName:    dl.liveName,
		title:       dl.title,
		avatarThumb: dl.avatarThumb,
		anchorOnly:  dl.anchorOnlyPageIdentity,
	}
	dl.mu.Unlock()
	if code == "" {
		code = statusCodeForSnapshot(info, isLive, known)
	}

	status := LiveStatus{
		Code:         code,
		LiveID:       info.liveID,
		RoomID:       info.roomID,
		UserUniqueID: info.pushID,
		LiveName:     info.liveName,
		Title:        info.title,
		AvatarThumb:  info.avatarThumb,
	}
	if known && code != LiveStatusUnknown && code != LiveStatusNotFound {
		status.Live = boolPointer(isLive)
		status.HasRoom = boolPointer(!info.anchorOnly && info.roomID != "")
		status.AccountOnly = boolPointer(info.anchorOnly)
	}
	return status
}
