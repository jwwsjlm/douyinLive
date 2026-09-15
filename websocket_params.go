package douyinLive

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
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
	// webcastPersistMsgCount 对齐 PC 客户端实测值：桌面客户端建连时上报 0。
	// webcastPersistMsgCount matches the observed PC client value: the desktop client sends 0 on connect.
	webcastPersistMsgCount = pcProtocolPersistMsgCount
	protobufContentType    = "protobuf"
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

// Joined returns the canonical comma-separated signing input.
// Joined 返回规范化的逗号分隔签名输入。
func (p websocketSignatureParams) Joined() string {
	fields := [...]struct{ key, value string }{
		{"live_id", p.LiveID},
		{"aid", p.AID},
		{"version_code", p.VersionCode},
		{"webcast_sdk_version", p.WebcastSDKVersion},
		{"room_id", p.RoomID},
		{"sub_room_id", p.SubRoomID},
		{"sub_channel_id", p.SubChannelID},
		{"did_rule", p.DidRule},
		{"user_unique_id", p.UserUniqueID},
		{"device_platform", p.DevicePlatform},
		{"device_type", p.DeviceType},
		{"ac", p.AC},
		{"identity", p.Identity},
	}
	var b strings.Builder
	for i, field := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(field.key)
		b.WriteByte('=')
		b.WriteString(field.value)
	}
	return b.String()
}

// XMSStub returns the MD5 stub derived from the canonical signing input.
// XMSStub 返回由规范化签名输入计算出的 MD5 stub。
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
	ScreenWidth    int
	ScreenHeight   int
	// PersistMsgCount 对应查询串中的 need_persist_msg_count，由协议画像决定。
	// 抓包实测 PC 客户端上报 0，Web 端沿用历史值 15。
	PersistMsgCount string
}

func newWebsocketURLParams(roomInfo roomInfoSnapshot, userAgent, cursor, internalExt, signature string) websocketURLParams {
	return newWebsocketURLParamsWithScreen(roomInfo, userAgent, cursor, internalExt, signature, defaultScreenWidth, defaultScreenHeight)
}

func newWebsocketURLParamsWithScreen(roomInfo roomInfoSnapshot, userAgent, cursor, internalExt, signature string, screenWidth, screenHeight int) websocketURLParams {
	return websocketURLParams{
		BrowserVersion:  browserVersionFromUserAgent(userAgent),
		Cursor:          cursor,
		InternalExt:     internalExt,
		UserUniqueID:    roomInfo.pushID,
		RoomID:          roomInfo.roomID,
		Signature:       signature,
		ScreenWidth:     screenWidth,
		ScreenHeight:    screenHeight,
		PersistMsgCount: webcastPersistMsgCount,
	}
}

func browserVersionFromUserAgent(userAgent string) string {
	if parts := strings.SplitN(userAgent, "Mozilla/", 2); len(parts) == 2 {
		return parts[1]
	}
	return userAgent
}

// QueryString returns the upstream WebSocket query string.
// QueryString 返回上游 WebSocket 查询字符串。
func (p websocketURLParams) QueryString() string {
	parts := []string{
		"app_name=" + webcastAppName,
		"version_code=" + webcastVersionCode,
		"webcast_sdk_version=" + webcastSDKVersion,
		"update_version_code=" + webcastSDKVersion,
		"compress=gzip",
		"device_platform=" + webcastDevice,
		"cookie_enabled=true",
		fmt.Sprintf("screen_width=%d", p.ScreenWidth),
		fmt.Sprintf("screen_height=%d", p.ScreenHeight),
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
		"need_persist_msg_count=" + p.PersistMsgCount,
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
	ScreenWidth    int
	ScreenHeight   int
}

func newInitialIMFetchParams(roomInfo roomInfoSnapshot, userAgent, msToken string) initialIMFetchParams {
	return newInitialIMFetchParamsWithScreen(roomInfo, userAgent, msToken, defaultScreenWidth, defaultScreenHeight)
}

func newInitialIMFetchParamsWithScreen(roomInfo roomInfoSnapshot, userAgent, msToken string, screenWidth, screenHeight int) initialIMFetchParams {
	return initialIMFetchParams{
		RoomID:         roomInfo.roomID,
		UserUniqueID:   roomInfo.pushID,
		BrowserVersion: browserVersionFromUserAgent(userAgent),
		MSToken:        msToken,
		ScreenWidth:    screenWidth,
		ScreenHeight:   screenHeight,
	}
}

// QueryString returns the initial IM fetch query string.
// QueryString 返回初始 IM fetch 查询字符串。
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
		fmt.Sprintf("screen_width=%d", p.ScreenWidth),
		fmt.Sprintf("screen_height=%d", p.ScreenHeight),
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
	pushURL, _ := websocketPushURLFromResponseWithSource(response)
	return pushURL
}

func websocketPushURLFromResponseWithSource(response interface {
	GetPushServerV2() string
	GetPushServer() string
	GetProxyServer() string
}) (string, string) {
	if response == nil {
		return "", ""
	}
	for _, candidate := range []struct {
		source string
		value  string
	}{
		{source: "push_server_v2", value: response.GetPushServerV2()},
		{source: "push_server", value: response.GetPushServer()},
		{source: "proxy_server", value: response.GetProxyServer()},
	} {
		if pushURL := normalizeWebsocketPushURL(candidate.value); pushURL != "" {
			return pushURL, candidate.source
		}
	}
	return "", ""
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
