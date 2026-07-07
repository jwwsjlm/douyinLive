package douyinLive

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/retry"
	"github.com/jwwsjlm/douyinLive/v2/sign"
	"github.com/tidwall/gjson"
)

// roomInfoSnapshot 保存房间信息的一致性快照。
// roomInfoSnapshot stores a consistent snapshot of room metadata.
type roomInfoSnapshot struct {
	liveID      string
	roomID      string
	pushID      string
	liveName    string
	title       string
	avatarThumb string
}

// GetName 返回直播间主播名称。
// GetName returns the live room owner name.
func (dl *DouyinLive) GetName() string {
	return dl.roomInfoSnapshot().liveName
}

// GetTitle 返回直播间标题。
// GetTitle returns the live room title.
func (dl *DouyinLive) GetTitle() string {
	return dl.roomInfoSnapshot().title
}

// GetAvatarThumb 返回主播头像缩略图地址。
// GetAvatarThumb returns the owner avatar thumbnail URL.
func (dl *DouyinLive) GetAvatarThumb() string {
	return dl.roomInfoSnapshot().avatarThumb
}

// roomInfoSnapshot 返回当前房间信息的线程安全快照。
// roomInfoSnapshot returns a thread-safe snapshot of the current room metadata.
func (dl *DouyinLive) roomInfoSnapshot() roomInfoSnapshot {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	return roomInfoSnapshot{
		liveID:      dl.liveID,
		roomID:      dl.roomID,
		pushID:      dl.pushID,
		liveName:    dl.liveName,
		title:       dl.title,
		avatarThumb: dl.avatarThumb,
	}
}

// updateRoomInfo 更新 WebSocket 和输出所需的房间信息。
// updateRoomInfo updates room metadata required by WebSocket signing and output.
func (dl *DouyinLive) updateRoomInfo(roomID, pushID, liveName, title, avatarThumb string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.roomID = roomID
	dl.pushID = pushID
	dl.liveName = liveName
	dl.title = title
	dl.avatarThumb = avatarThumb
}

func (dl *DouyinLive) updateRoomInfoFromEnter(info roomInfoSnapshot) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.roomID = info.roomID
	if dl.pushID == "" {
		dl.pushID = info.pushID
	}
	dl.liveName = info.liveName
	dl.title = info.title
	dl.avatarThumb = info.avatarThumb
}

func (dl *DouyinLive) setLivePageIDs(roomID, userUniqueID string) {
	roomID = strings.TrimSpace(roomID)
	userUniqueID = strings.TrimSpace(userUniqueID)
	if roomID == "" && userUniqueID == "" {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if roomID != "" {
		dl.roomID = roomID
	}
	if userUniqueID != "" {
		dl.pushID = userUniqueID
	}
}

// parseRoomInfo 从 web/enter 响应中提取房间 ID、push ID 和展示信息。
// parseRoomInfo extracts room ID, push ID, and display metadata from a web/enter response.
func parseRoomInfo(body string) (roomInfoSnapshot, error) {
	roomID := firstNonEmptyGJSON(body,
		"data.data.0.id_str",
		"data.data.0.id",
		"data.room.id_str",
		"data.room.id",
		"data.enter_room_id",
	)
	pushID := firstNonEmptyGJSON(body,
		"data.user.id_str",
		"data.user.id",
		"data.data.0.owner_user_id_str",
		"data.data.0.owner.id_str",
		"data.data.0.owner.id",
		"data.room.owner_user_id_str",
		"data.room.owner.id_str",
		"data.room.owner.id",
	)
	liveName := firstNonEmptyGJSON(body,
		"data.user.nickname",
		"data.data.0.owner.nickname",
		"data.room.owner.nickname",
	)
	avatarThumb := firstNonEmptyGJSON(body,
		"data.user.avatar_thumb.url_list.2",
		"data.user.avatar_thumb.url_list.0",
		"data.data.0.owner.avatar_thumb.url_list.2",
		"data.data.0.owner.avatar_thumb.url_list.0",
		"data.room.owner.avatar_thumb.url_list.2",
		"data.room.owner.avatar_thumb.url_list.0",
	)

	if roomID == "" || pushID == "" {
		return roomInfoSnapshot{}, errors.New("无法提取房间信息")
	}

	return roomInfoSnapshot{
		roomID:      roomID,
		pushID:      pushID,
		liveName:    liveName,
		title:       firstNonEmptyGJSON(body, "data.data.0.title", "data.room.title"),
		avatarThumb: avatarThumb,
	}, nil
}

// firstNonEmptyGJSON 按路径顺序返回第一个非空 gjson 字符串值。
// firstNonEmptyGJSON returns the first non-empty gjson string value for the given paths.
func parseRoomIDFromLivePage(body string) string {
	for _, marker := range []string{
		`"room":{"id_str":"`,
		`\"room\":{\"id_str\":\"`,
		`"room":{"id":`,
		`\"room\":{\"id\":`,
		"room_id=",
		"room_id%3D",
		`"room_id":"`,
		`\"room_id\":\"`,
		`"room_id":`,
		`"room_id_str":"`,
		`\"room_id_str\":\"`,
		`"roomId":"`,
		`\"roomId\":\"`,
		`"roomId":`,
	} {
		if value := digitsAfterMarker(body, marker); value != "" {
			return value
		}
	}
	return ""
}

func parseUserUniqueIDFromLivePage(body string) string {
	for _, marker := range []string{
		"user_unique_id=",
		"user_unique_id%3D",
		`"user_unique_id":"`,
		`\"user_unique_id\":\"`,
		`"user_unique_id":`,
	} {
		if value := digitsAfterMarker(body, marker); value != "" {
			return value
		}
	}
	return ""
}

func parseLiveStatusFromLivePage(body, roomID string) (bool, bool) {
	if roomID == "" {
		return false, false
	}
	idx := strings.Index(body, roomID)
	if idx < 0 {
		return false, false
	}
	end := idx + 1200
	if end > len(body) {
		end = len(body)
	}
	segment := body[idx:end]
	switch {
	case strings.Contains(segment, `"status":2`) || strings.Contains(segment, `\"status\":2`):
		return true, true
	case strings.Contains(segment, `"status":`) || strings.Contains(segment, `\"status\":`):
		return false, true
	default:
		return false, false
	}
}

func digitsAfterMarker(body, marker string) string {
	idx := strings.Index(body, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	for start < len(body) && (body[start] == '"' || body[start] == '\'' || body[start] == ':' || body[start] == ' ') {
		start++
	}
	end := start
	for end < len(body) && body[end] >= '0' && body[end] <= '9' {
		end++
	}
	if end-start < 10 {
		return ""
	}
	return body[start:end]
}

func firstNonEmptyGJSON(body string, paths ...string) string {
	for _, path := range paths {
		value := gjson.Get(body, path).String()
		if value != "" {
			return value
		}
	}
	return ""
}

// fetchRoomEnterData 获取直播间接口数据（对齐 DouyinLiveRecorder 的 web/enter 逻辑）
// fetchRoomEnterData 获取直播间入口数据，优先使用短时缓存。
// fetchRoomEnterData fetches room enter data and prefers the short-lived cache.
func (dl *DouyinLive) fetchLivePageState() error {
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID != "" && roomInfo.pushID != "" {
		return nil
	}
	ctx, cancel := dl.requestContext()
	defer cancel()

	headers := map[string]string{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		"Accept-Encoding": "identity",
		"Cookie":          dl.getCookieString(),
		"Referer":         "https://live.douyin.com/",
		"User-Agent":      dl.userAgent,
	}
	for key, value := range browserClientHintHeaders(dl.userAgent) {
		headers[key] = value
	}

	resp, err := dl.client.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get("https://live.douyin.com/" + dl.liveID)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("empty live page response")
	}
	body, err := responseString(resp)
	if err != nil {
		return err
	}
	roomID := parseRoomIDFromLivePage(body)
	userUniqueID := parseUserUniqueIDFromLivePage(body)
	if roomID == "" && userUniqueID == "" {
		return errors.New("live page state not found")
	}
	dl.setLivePageIDs(roomID, userUniqueID)
	if isLive, ok := parseLiveStatusFromLivePage(body, roomID); ok {
		dl.setLiveStatus(isLive)
	}
	dl.logger.Debug("从直播间页面预取状态成功", "live_id", dl.liveID, "room_id", roomID, "user_unique_id", userUniqueID)
	return nil
}

func (dl *DouyinLive) fetchRoomEnterData() (string, error) {
	V, found := dl.ristretto.Get(dl.liveID)
	if found {
		dl.logger.Debug("从缓存获取直播间信息", "live_id", dl.liveID)
		roomInfo, err := parseRoomInfo(V)
		if err != nil {
			return "", err
		}
		dl.updateRoomInfoFromEnter(roomInfo)
		return V, nil
	}

	return dl.refreshRoomEnterData()
}

// refreshRoomEnterData 强制请求直播间入口数据并刷新房间信息。
// refreshRoomEnterData force-fetches room enter data and refreshes room metadata.
func (dl *DouyinLive) refreshRoomEnterData() (string, error) {
	var body string

	dl.logger.Debug("开始请求直播间信息", "live_id", dl.liveID)
	if err := dl.fetchLivePageState(); err != nil {
		dl.logger.Debug("从直播间页面预取状态失败，继续请求 web/enter", "live_id", dl.liveID, "err", err)
	}
	err := retry.Do(
		func() error {
			// 核心请求逻辑
			reqBody, err := dl.doRequest()
			if err != nil {
				return err
			}
			body = reqBody
			return nil
		},
		retry.Attempts(3),          // 最多重试 3 次
		retry.Delay(1*time.Second), // 每次重试延迟1秒
		retry.RetryIf(dl.shouldRetryRoomEnter),
	)

	if err != nil {
		if fallbackBody, ok := dl.roomEnterFallbackBody(err); ok {
			dl.logger.Debug("web/enter 返回空响应，使用直播间页面状态兜底", "live_id", dl.liveID, "err", err)
			body = fallbackBody
		} else {
			dl.logger.Error("请求直播间信息失败，重试结束", "live_id", dl.liveID, "err", err)
			return "", err
		}
	}

	roomInfo, err := parseRoomInfo(body)
	if err != nil {
		dl.logRoomInfoResponseSummary(body)
		return "", err
	}

	dl.updateRoomInfoFromEnter(roomInfo)
	dl.ristretto.SetWithTTL(dl.liveID, body, 1, 5*time.Second) // 将结果缓存到 Ristretto，成本为 1

	return body, nil
}

func (dl *DouyinLive) buildRoomEnterParams() string {
	roomInfo := dl.roomInfoSnapshot()
	parts := []string{
		"aid=" + webcastAid,
		"app_name=" + webcastAppName,
		"live_id=" + webcastLiveID,
		"device_platform=" + webcastDevice,
		"language=zh-CN",
		"enter_from=link_share",
		"cookie_enabled=true",
		fmt.Sprintf("screen_width=%d", defaultScreenWidth),
		fmt.Sprintf("screen_height=%d", defaultScreenHeight),
		"browser_language=zh-CN",
		"browser_platform=Win32",
		"browser_name=Chrome",
		"browser_version=" + queryEscapeValue(chromeVersionFromUserAgent(dl.userAgent)),
		"os_name=Windows",
		"os_version=10",
		"web_rid=" + queryEscapeValue(dl.liveID),
		"room_id_str=" + queryEscapeValue(roomInfo.roomID),
		"enter_source=",
		"is_need_double_stream=false",
		"insert_task_id=",
		"live_reason=",
		"msToken=" + queryEscapeValue(dl.initialIMFetchMSToken()),
	}
	return strings.Join(parts, "&")
}

func (dl *DouyinLive) doRequest() (string, error) {
	params := dl.buildRoomEnterParams()
	// 参考代码 https://github.com/ihmily/DouyinLiveRecorder
	headers := map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"Accept-Encoding": "identity",
		"Cookie":          dl.getCookieString(),
		"Referer":         "https://live.douyin.com/" + dl.liveID,
		"User-Agent":      dl.userAgent,
	}
	for key, value := range browserClientHintHeaders(dl.userAgent) {
		headers[key] = value
	}
	aBogus := sign.AbSign(params, dl.userAgent)
	url := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, queryEscapeURLSearchParamsValue(aBogus))
	roomInfo := dl.roomInfoSnapshot()
	dl.logger.Debug("请求直播间 web/enter",
		logFlowArgs("room_info", "web_enter",
			"live_id", dl.liveID,
			"room_id", roomInfo.roomID,
			"endpoint", "/webcast/room/web/enter/",
			"query_len", len(params),
			"abogus_len", len(aBogus),
		)...,
	)
	ctx, cancel := dl.requestContext()
	defer cancel()

	resp, err := dl.client.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get(url)
	if err != nil {
		body := ""
		if resp != nil {
			body, _ = responseString(resp)
		}
		return body, fmt.Errorf("请求直播间信息失败: %w", err)
	}
	if resp == nil {
		return "", errRoomInfoEmpty
	}

	body, err := responseString(resp)
	if err != nil {
		return "", fmt.Errorf("读取直播间信息响应失败: %w", err)
	}

	if body == "" {
		return "", fmt.Errorf("%w status=%d content_type=%q content_length=%d raw_len=%d",
			errRoomInfoEmpty,
			resp.GetStatusCode(),
			resp.GetHeader("Content-Type"),
			resp.ContentLength,
			len(resp.Bytes()),
		)
	}
	dl.logger.Debug("直播间 web/enter 响应成功",
		logFlowArgs("room_info", "web_enter",
			"live_id", dl.liveID,
			"room_id", roomInfo.roomID,
			"status", resp.GetStatusCode(),
			"content_type", resp.GetHeader("Content-Type"),
			"raw_len", len(resp.Bytes()),
		)...,
	)

	if statusCode := gjson.Get(body, "status_code").Int(); statusCode != 0 {
		return "", fmt.Errorf("直播间信息接口返回异常 status_code=%d", statusCode)
	}

	return body, nil
}

func shouldRetryRoomEnter(err error) bool {
	return isRoomInfoEmptyError(err)
}

func (dl *DouyinLive) shouldRetryRoomEnter(err error) bool {
	if !shouldRetryRoomEnter(err) {
		return false
	}
	_, canFallback := dl.roomEnterFallbackBody(err)
	return !canFallback
}

func isRoomInfoEmptyError(err error) bool {
	return errors.Is(err, errRoomInfoEmpty) || (err != nil && strings.Contains(err.Error(), errRoomInfoEmpty.Error()))
}

func (dl *DouyinLive) roomEnterFallbackBody(err error) (string, bool) {
	if !isRoomInfoEmptyError(err) || !dl.isLiveStatus() {
		return "", false
	}
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID == "" || roomInfo.pushID == "" {
		return "", false
	}
	return fmt.Sprintf(
		`{"status_code":0,"data":{"data":[{"id_str":%q,"status":2,"owner":{"id_str":%q,"nickname":%q},"title":%q}]}}`,
		roomInfo.roomID,
		roomInfo.pushID,
		roomInfo.liveName,
		roomInfo.title,
	), true
}

// logRoomInfoResponseSummary 输出无法解析房间信息时的响应摘要。
// logRoomInfoResponseSummary logs a response summary when room metadata cannot be parsed.
func (dl *DouyinLive) logRoomInfoResponseSummary(body string) {
	if dl.logger == nil {
		return
	}
	dl.logger.Warn("直播间信息响应无法提取房间参数",
		"live_id", dl.liveID,
		"body_len", len(body),
		"status_code", gjson.Get(body, "status_code").Int(),
		"message", firstNonEmptyGJSON(body, "message", "prompts", "extra.log_pb.impr_id"),
		"has_data_data_0", gjson.Get(body, "data.data.0").Exists(),
		"has_enter_room_id", gjson.Get(body, "data.enter_room_id").Exists(),
		"has_user", gjson.Get(body, "data.user").Exists(),
	)
}

// refreshLiveStatusFromAPI 通过房间接口刷新当前直播状态。
// refreshLiveStatusFromAPI 通过 HTTP 接口刷新并保存直播状态。
// refreshLiveStatusFromAPI refreshes and stores live status through the HTTP API.
func (dl *DouyinLive) refreshLiveStatusFromAPI() (bool, error) {
	isLive, err := dl.fetchLiveStatusFromAPI()
	if err != nil {
		return false, err
	}
	dl.setLiveStatus(isLive)
	return isLive, nil
}

// fetchLiveStatusFromAPI 从直播间入口数据判断当前是否开播。
// fetchLiveStatusFromAPI determines whether the room is live from room enter data.
func (dl *DouyinLive) fetchLiveStatusFromAPI() (bool, error) {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()

	if err := dl.prepareRequestContextLocked(); err != nil {
		return false, err
	}

	body, err := dl.refreshRoomEnterData()
	if err != nil {
		if isRoomInfoEmptyError(err) && dl.isLiveStatus() {
			roomInfo := dl.roomInfoSnapshot()
			if roomInfo.roomID != "" && roomInfo.pushID != "" {
				return true, nil
			}
		}
		return false, err
	}

	status := gjson.Get(body, "data.data.0.status").Int()
	return status == 2, nil
}

// IsLive 检查直播间是否开播，并返回判活过程中的错误。
// IsLive 检查直播间当前是否开播。
// IsLive checks whether the live room is currently live.
func (dl *DouyinLive) IsLive() (bool, error) {
	isLive, err := dl.refreshLiveStatusFromAPI()
	if err != nil {
		dl.setLiveStatus(false)
		return false, err
	}
	return isLive, nil
}
