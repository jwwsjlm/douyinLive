package douyinLive

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/elliotchance/orderedmap"
)

const (
	webcastAppName      = "douyin_web"
	webcastVersionCode  = "180800"
	webcastAid          = "6383"
	webcastLiveID       = "1"
	webcastDidRule      = "3"
	webcastDevice       = "web"
	webcastIdentity     = "audience"
	webcastIMPath       = "/webcast/im/fetch/"
	webcastPushPath     = "/webcast/im/push/v2/"
	webcastEndpoint     = "live_pc"
	webcastHost         = "https://live.douyin.com"
	defaultScreenWidth  = 1920
	defaultScreenHeight = 1080
	defaultCursor       = "d-1_u-1_fh-7383731312643626035_t-1719159695790_r-1"
	defaultWRDSVersion  = "7382620942951772256"
	protobufContentType = "protobuf"
)

type websocketSignatureParams struct {
	LiveID            string
	AID               string
	VersionCode       string
	WebcastSDKVersion string
	RoomID            string
	SubRoomID         string
	SubChannelID      string
	DidRule           string
	UserUniqueID      string
	DevicePlatform    string
	DeviceType        string
	AC                string
	Identity          string
}

func newWebsocketSignatureParams(roomID, userUniqueID string) websocketSignatureParams {
	return websocketSignatureParams{
		LiveID:            webcastLiveID,
		AID:               webcastAid,
		VersionCode:       webcastVersionCode,
		WebcastSDKVersion: webcastSDKVersion,
		RoomID:            roomID,
		SubRoomID:         "",
		SubChannelID:      "",
		DidRule:           webcastDidRule,
		UserUniqueID:      userUniqueID,
		DevicePlatform:    webcastDevice,
		DeviceType:        "",
		AC:                "",
		Identity:          webcastIdentity,
	}
}

func (p websocketSignatureParams) OrderedMap() *orderedmap.OrderedMap {
	m := orderedmap.NewOrderedMap()
	m.Set("live_id", p.LiveID)
	m.Set("aid", p.AID)
	m.Set("version_code", p.VersionCode)
	m.Set("webcast_sdk_version", p.WebcastSDKVersion)
	m.Set("room_id", p.RoomID)
	m.Set("sub_room_id", p.SubRoomID)
	m.Set("sub_channel_id", p.SubChannelID)
	m.Set("did_rule", p.DidRule)
	m.Set("user_unique_id", p.UserUniqueID)
	m.Set("device_platform", p.DevicePlatform)
	m.Set("device_type", p.DeviceType)
	m.Set("ac", p.AC)
	m.Set("identity", p.Identity)
	return m
}

func (p websocketSignatureParams) Joined() string {
	var b strings.Builder
	m := p.OrderedMap()
	for i, key := range m.Keys() {
		if i > 0 {
			b.WriteByte(',')
		}
		value, _ := m.Get(key)
		b.WriteString(fmt.Sprintf("%s=%s", key, value))
	}
	return b.String()
}

func (p websocketSignatureParams) XMSStub() string {
	sum := md5.Sum([]byte(p.Joined()))
	return hex.EncodeToString(sum[:])
}

type websocketURLParams struct {
	BrowserVersion string
	Cursor         string
	InternalExt    string
	UserUniqueID   string
	RoomID         string
	Signature      string
}

func newWebsocketURLParams(roomInfo roomInfoSnapshot, userAgent, cursor, internalExt, signature string) websocketURLParams {
	return websocketURLParams{
		BrowserVersion: browserVersionFromUserAgent(userAgent),
		Cursor:         cursor,
		InternalExt:    internalExt,
		UserUniqueID:   roomInfo.pushID,
		RoomID:         roomInfo.roomID,
		Signature:      signature,
	}
}

func browserVersionFromUserAgent(userAgent string) string {
	if parts := strings.SplitN(userAgent, "Mozilla/", 2); len(parts) == 2 {
		return parts[1]
	}
	return userAgent
}

func (p websocketURLParams) QueryString() string {
	parts := []string{
		"app_name=" + webcastAppName,
		"version_code=" + webcastVersionCode,
		"webcast_sdk_version=" + webcastSDKVersion,
		"update_version_code=" + webcastSDKVersion,
		"compress=gzip",
		"device_platform=" + webcastDevice,
		"cookie_enabled=true",
		fmt.Sprintf("screen_width=%d", defaultScreenWidth),
		fmt.Sprintf("screen_height=%d", defaultScreenHeight),
		"browser_language=zh-CN",
		"browser_platform=Win32",
		"browser_name=Mozilla",
		"browser_version=" + websocketQueryValue(p.BrowserVersion),
		"browser_online=true",
		"tz_name=Asia/Shanghai",
		"cursor=" + p.Cursor,
		"internal_ext=" + p.InternalExt,
		"host=" + webcastHost,
		"aid=" + webcastAid,
		"live_id=" + webcastLiveID,
		"did_rule=" + webcastDidRule,
		"endpoint=" + webcastEndpoint,
		"support_wrds=1",
		"user_unique_id=" + p.UserUniqueID,
		"im_path=" + webcastIMPath,
		"identity=" + webcastIdentity,
		"need_persist_msg_count=15",
		"insert_task_id=",
		"live_reason=",
		"room_id=" + p.RoomID,
		"heartbeatDuration=0",
		"signature=" + websocketQueryValue(p.Signature),
	}
	return strings.Join(parts, "&")
}

func websocketQueryValue(value string) string {
	return strings.ReplaceAll(value, " ", "%20")
}

type initialIMFetchParams struct {
	RoomID         string
	UserUniqueID   string
	BrowserVersion string
	MSToken        string
}

func newInitialIMFetchParams(roomInfo roomInfoSnapshot, userAgent, msToken string) initialIMFetchParams {
	return initialIMFetchParams{
		RoomID:         roomInfo.roomID,
		UserUniqueID:   roomInfo.pushID,
		BrowserVersion: browserVersionFromUserAgent(userAgent),
		MSToken:        msToken,
	}
}

func (p initialIMFetchParams) QueryString() string {
	parts := []string{
		"resp_content_type=" + protobufContentType,
		"did_rule=" + webcastDidRule,
		"device_id=",
		"app_name=" + webcastAppName,
		"endpoint=" + webcastEndpoint,
		"support_wrds=1",
		"user_unique_id=" + queryEscapeURLSearchParamsValue(p.UserUniqueID),
		"identity=" + webcastIdentity,
		"need_persist_msg_count=15",
		"insert_task_id=",
		"live_reason=",
		"room_id=" + queryEscapeURLSearchParamsValue(p.RoomID),
		"version_code=" + webcastVersionCode,
		"last_rtt=0",
		"live_id=" + webcastLiveID,
		"aid=" + webcastAid,
		"fetch_rule=1",
		"cursor=",
		"internal_ext=",
		"device_platform=" + webcastDevice,
		"cookie_enabled=true",
		fmt.Sprintf("screen_width=%d", defaultScreenWidth),
		fmt.Sprintf("screen_height=%d", defaultScreenHeight),
		"browser_language=zh-CN",
		"browser_platform=Win32",
		"browser_name=Mozilla",
		"browser_version=" + queryEscapeURLSearchParamsValue(p.BrowserVersion),
		"browser_online=true",
		"tz_name=Asia/Shanghai",
		"msToken=" + queryEscapeURLSearchParamsValue(p.MSToken),
	}
	return strings.Join(parts, "&")
}

func defaultInternalExt(roomID, userUniqueID string, nowMs int64) string {
	return fmt.Sprintf(
		"internal_src:dim|wss_push_room_id:%s|wss_push_did:%s|first_req_ms:%d|fetch_time:%d|seq:1|wss_info:0-%d-0-0|wrds_v:%s",
		roomID,
		userUniqueID,
		nowMs,
		nowMs,
		nowMs,
		defaultWRDSVersion,
	)
}

func websocketPushURLFromResponse(response interface {
	GetPushServerV2() string
	GetPushServer() string
	GetProxyServer() string
}) string {
	if response == nil {
		return ""
	}
	for _, candidate := range []string{
		response.GetPushServerV2(),
		response.GetPushServer(),
		response.GetProxyServer(),
	} {
		if pushURL := normalizeWebsocketPushURL(candidate); pushURL != "" {
			return pushURL
		}
	}
	return ""
}

func normalizeWebsocketPushURL(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	if idx := strings.IndexAny(candidate, ",;"); idx >= 0 {
		candidate = strings.TrimSpace(candidate[:idx])
	}
	candidate = strings.TrimRight(candidate, "/")

	switch {
	case strings.HasPrefix(candidate, "wss://") || strings.HasPrefix(candidate, "ws://"):
	case strings.HasPrefix(candidate, "https://"):
		candidate = "wss://" + strings.TrimPrefix(candidate, "https://")
	case strings.HasPrefix(candidate, "http://"):
		candidate = "ws://" + strings.TrimPrefix(candidate, "http://")
	default:
		candidate = "wss://" + candidate
	}

	if strings.Contains(candidate, strings.TrimRight(webcastPushPath, "/")) {
		return candidate + "/"
	}
	return candidate + webcastPushPath
}
