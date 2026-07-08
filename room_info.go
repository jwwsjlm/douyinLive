package douyinLive

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/codeGROOVE-dev/retry"
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
	anchorOnly  bool
}

var livePageRoomStatusPattern = regexp.MustCompile(`"id_str"\s*:\s*"?(\d+)"?\s*,\s*"status"\s*:\s*(\d+)\s*,\s*"status_str"\s*:\s*"?([^",}]*)"?`)

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

// HasAnchorOnlyPageIdentity ????????? roomInfo.anchor ??? roomInfo.room?
// HasAnchorOnlyPageIdentity reports whether the page returned roomInfo.anchor without roomInfo.room.
func (dl *DouyinLive) HasAnchorOnlyPageIdentity() bool {
	return dl.roomInfoSnapshot().anchorOnly
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
		anchorOnly:  dl.anchorOnlyPageIdentity,
	}
}

// updateRoomInfo 更新 WebSocket 和输出所需的房间信息。
// updateRoomInfo updates room metadata required by WebSocket signing and output.
// 参数/Parameters:
//   - roomID: 抖音长房间 ID。 Douyin long room ID.
//   - pushID: WebSocket 签名所需的用户唯一 ID。 User unique ID required for WebSocket signing.
//   - liveName: 主播昵称。 Live owner nickname.
//   - title: 直播间标题。 Live room title.
//   - avatarThumb: 主播头像缩略图地址。 Live owner avatar thumbnail URL.
func (dl *DouyinLive) updateRoomInfo(roomID, pushID, liveName, title, avatarThumb string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.roomID = roomID
	dl.pushID = pushID
	dl.liveName = liveName
	dl.title = title
	dl.avatarThumb = avatarThumb
	dl.anchorOnlyPageIdentity = roomID == "" && (liveName != "" || avatarThumb != "")
}

func (dl *DouyinLive) updateRoomInfoFromEnter(info roomInfoSnapshot) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if info.roomID != "" {
		dl.roomID = info.roomID
		dl.anchorOnlyPageIdentity = false
	}
	if info.anchorOnly {
		dl.roomID = ""
		dl.anchorOnlyPageIdentity = true
	}
	if dl.pushID == "" {
		dl.pushID = info.pushID
	}
	if info.liveName != "" {
		dl.liveName = info.liveName
	}
	if info.title != "" {
		dl.title = info.title
	}
	if info.avatarThumb != "" {
		dl.avatarThumb = info.avatarThumb
	}
}

// updateRoomInfoFromLivePage 合并直播页内嵌状态中的房间信息。
// updateRoomInfoFromLivePage merges room metadata parsed from the embedded live-page state.
// 参数/Parameters:
//   - info: 从直播页 HTML 解析出的房间信息。 Room metadata parsed from the live page HTML.
func (dl *DouyinLive) updateRoomInfoFromLivePage(info roomInfoSnapshot) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if info.anchorOnly {
		dl.roomID = ""
		dl.anchorOnlyPageIdentity = true
	} else if info.roomID != "" {
		dl.roomID = info.roomID
		dl.anchorOnlyPageIdentity = false
	}
	if info.pushID != "" {
		dl.pushID = info.pushID
	}
	if info.liveName != "" {
		dl.liveName = info.liveName
	}
	if info.title != "" {
		dl.title = info.title
	}
	if info.avatarThumb != "" {
		dl.avatarThumb = info.avatarThumb
	}
}

// logMissingLiveName 记录已拿到连接必需参数但缺少主播昵称的情况。
// logMissingLiveName records when required connection parameters are available but the owner nickname is missing.
// 参数/Parameters:
//   - source: 触发缺失日志的数据来源。 Data source that triggered the missing-name log.
//   - info: 当前房间信息快照。 Current room metadata snapshot.
func (dl *DouyinLive) logMissingLiveName(source string, info roomInfoSnapshot) {
	if dl.logger == nil {
		return
	}
	dl.logger.Warn("直播间名称未获取到，已继续连接",
		"live_id", dl.liveID,
		"room_id", info.roomID,
		"user_unique_id", info.pushID,
		"source", source,
	)
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
// 参数/Parameters:
//   - body: web/enter 接口返回体。 Response body returned by the web/enter endpoint.
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
		"data.user.avatar_thumb.url_list.1",
		"data.user.avatar_thumb.url_list.0",
		"data.data.0.owner.avatar_thumb.url_list.2",
		"data.data.0.owner.avatar_thumb.url_list.1",
		"data.data.0.owner.avatar_thumb.url_list.0",
		"data.room.owner.avatar_thumb.url_list.2",
		"data.room.owner.avatar_thumb.url_list.1",
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

// parseRoomInfoFromLivePage 从直播页 SSR 内嵌状态中提取房间展示信息。
// parseRoomInfoFromLivePage extracts display metadata from the live page's embedded SSR state.
// 参数/Parameters:
//   - body: 直播间页面 HTML 内容。 Live-room page HTML content.
//
// parseRoomInfoFromLivePage ???? SSR ??????????????
// parseRoomInfoFromLivePage extracts display metadata from the live page's embedded SSR state.
// ??/Parameters:
//   - body: ????? HTML ??? Live-room page HTML content.
func parseRoomInfoFromLivePage(body string) roomInfoSnapshot {
	for _, candidate := range livePageStateCandidates(body) {
		if roomInfoObj := roomInfoObjectFromLivePageState(candidate); roomInfoObj != "" {
			hasRoomObj := roomInfoObjectHasRoomIdentity(roomInfoObj)
			hasAnchorObj := roomInfoObjectHasAnchorIdentity(roomInfoObj)
			roomID := firstNonEmptyGJSON(roomInfoObj,
				"room.id_str",
				"room.id",
			)
			info := roomInfoSnapshot{
				roomID: roomID,
				pushID: firstNonEmptyGJSON(roomInfoObj,
					"anchor.id_str",
					"anchor.id",
					"room.owner.id_str",
					"room.owner.id",
					"room.owner_user_id_str",
					"room.owner_user_id",
					"owner.id_str",
					"owner.id",
				),
				liveName: firstNonEmptyGJSON(roomInfoObj,
					"anchor.nickname",
					"room.owner.nickname",
					"owner.nickname",
				),
				title: firstNonEmptyGJSON(roomInfoObj,
					"room.title",
					"title",
				),
				avatarThumb: firstNonEmptyGJSON(roomInfoObj,
					"anchor.avatar_thumb.url_list.2",
					"anchor.avatar_thumb.url_list.1",
					"anchor.avatar_thumb.url_list.0",
					"room.owner.avatar_thumb.url_list.2",
					"room.owner.avatar_thumb.url_list.1",
					"room.owner.avatar_thumb.url_list.0",
					"owner.avatar_thumb.url_list.2",
					"owner.avatar_thumb.url_list.1",
					"owner.avatar_thumb.url_list.0",
				),
				anchorOnly: hasAnchorObj && !hasRoomObj,
			}
			if info.roomID != "" || info.pushID != "" || info.liveName != "" || info.title != "" || info.avatarThumb != "" || info.anchorOnly {
				return info
			}
		}

		roomObj := roomObjectFromLivePageState(candidate)
		if roomObj == "" {
			continue
		}
		info := roomInfoSnapshot{
			roomID: firstNonEmptyGJSON(roomObj,
				"id_str",
				"id",
			),
			pushID: firstNonEmptyGJSON(roomObj,
				"owner.id_str",
				"owner.id",
				"owner_user_id_str",
				"owner_user_id",
			),
			liveName: firstNonEmptyGJSON(roomObj,
				"owner.nickname",
				"anchor.nickname",
			),
			title: firstNonEmptyGJSON(roomObj,
				"title",
			),
			avatarThumb: firstNonEmptyGJSON(roomObj,
				"owner.avatar_thumb.url_list.2",
				"owner.avatar_thumb.url_list.1",
				"owner.avatar_thumb.url_list.0",
				"anchor.avatar_thumb.url_list.2",
				"anchor.avatar_thumb.url_list.1",
				"anchor.avatar_thumb.url_list.0",
			),
		}
		if info.roomID != "" || info.pushID != "" || info.liveName != "" || info.title != "" || info.avatarThumb != "" {
			return info
		}
	}
	return roomInfoSnapshot{}
}

func livePageStateCandidates(body string) []string {
	candidates := []string{body}
	decoded := body
	for i := 0; i < 6; i++ {
		next := strings.NewReplacer(
			`\\u0026`, "&",
			`\u0026`, "&",
			`\\\"`, `"`,
			`\"`, `"`,
		).Replace(decoded)
		if next == decoded {
			break
		}
		decoded = next
		candidates = append(candidates, decoded)
	}
	return candidates
}

func roomInfoObjectFromLivePageState(body string) string {
	var anchorOnlyFallback string
	for _, marker := range []string{
		`"roomStore":{"roomInfo":`,
		`"roomInfo":`,
	} {
		searchFrom := 0
		for {
			relativeMarkerIdx := strings.Index(body[searchFrom:], marker)
			if relativeMarkerIdx < 0 {
				break
			}
			markerIdx := searchFrom + relativeMarkerIdx
			openIdx := strings.Index(body[markerIdx+len(marker):], "{")
			if openIdx < 0 {
				searchFrom = markerIdx + len(marker)
				continue
			}
			openIdx += markerIdx + len(marker)
			obj := jsonObjectAt(body, openIdx)
			if obj != "" {
				if roomInfoObjectHasRoomIdentity(obj) {
					return obj
				}
				if anchorOnlyFallback == "" && roomInfoObjectHasAnchorIdentity(obj) {
					anchorOnlyFallback = obj
				}
			}
			searchFrom = markerIdx + len(marker)
		}
	}
	return anchorOnlyFallback
}

func roomInfoObjectHasIdentity(obj string) bool {
	return roomInfoObjectHasRoomIdentity(obj) || roomInfoObjectHasAnchorIdentity(obj)
}

func roomInfoObjectHasRoomIdentity(obj string) bool {
	return firstNonEmptyGJSON(obj, "room.id_str", "room.id") != ""
}

func roomInfoObjectHasAnchorIdentity(obj string) bool {
	if !jsonObjectExists(obj, "anchor") {
		return false
	}
	if firstNonEmptyGJSON(obj,
		"anchor.id_str",
		"anchor.id",
		"anchor.sec_uid",
		"anchor.nickname",
		"anchor.avatar_thumb.url_list.0",
	) != "" {
		return true
	}
	return strings.TrimSpace(gjson.Get(obj, "anchor").Raw) != "{}"
}

func jsonObjectExists(obj, path string) bool {
	raw := strings.TrimSpace(gjson.Get(obj, path).Raw)
	return strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}")
}

func roomObjectFromLivePageState(body string) string {
	for _, marker := range []string{
		`"roomStore":{"roomInfo":{"room":`,
		`"roomInfo":{"room":`,
		`"room":`,
	} {
		markerIdx := strings.Index(body, marker)
		if markerIdx < 0 {
			continue
		}
		openIdx := strings.Index(body[markerIdx+len(marker):], "{")
		if openIdx < 0 {
			continue
		}
		openIdx += markerIdx + len(marker)
		if obj := jsonObjectAt(body, openIdx); obj != "" && gjson.Get(obj, "id_str").String() != "" {
			return obj
		}
	}
	return ""
}

func jsonObjectAt(body string, openIdx int) string {
	if openIdx < 0 || openIdx >= len(body) || body[openIdx] != '{' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for idx := openIdx; idx < len(body); idx++ {
		ch := body[idx]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[openIdx : idx+1]
			}
		}
	}
	return ""
}

// parseRoomIDFromLivePage 从直播间 HTML 中提取房间 ID。
// parseRoomIDFromLivePage extracts the room ID from a live-room HTML page.
// 参数/Parameters:
//   - body: 直播间页面 HTML 内容。 Live-room page HTML content.
func parseRoomIDFromLivePage(body string) string {
	for _, candidate := range livePageStateCandidates(body) {
		for _, match := range livePageRoomStatusPattern.FindAllStringSubmatch(candidate, -1) {
			if len(match) >= 2 && match[1] != "" {
				return match[1]
			}
		}
	}
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
		"gift_effect_bg_",
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

type livePageStateSnapshot struct {
	info              roomInfoSnapshot
	userUniqueID      string
	hasAnchorIdentity bool
	isLive            bool
	statusKnown       bool
}

func (s livePageStateSnapshot) hasRoomIdentity() bool {
	return strings.TrimSpace(s.info.roomID) != ""
}

func (s livePageStateSnapshot) hasKnownPageIdentity() bool {
	return s.hasRoomIdentity() || s.hasAnchorIdentity
}

// parseLivePageState 汇总直播页 SSR 状态。
// 注意：网页里出现 user_unique_id 只能说明浏览器/访问者身份存在，不能证明直播间存在。
// 只有解析到 room_id/id_str/gift_effect_bg_ 等房间身份后，才把页面视为有效直播间页面。
func parseLivePageState(body string) livePageStateSnapshot {
	state := livePageStateSnapshot{
		info: parseRoomInfoFromLivePage(body),
	}
	state.hasAnchorIdentity = state.info.anchorOnly ||
		strings.TrimSpace(state.info.liveName) != "" ||
		strings.TrimSpace(state.info.avatarThumb) != "" ||
		(state.info.roomID == "" && strings.TrimSpace(state.info.pushID) != "")
	if state.info.roomID == "" && !state.info.anchorOnly {
		state.info.roomID = parseRoomIDFromLivePage(body)
	}
	state.userUniqueID = parseUserUniqueIDFromLivePage(body)
	if state.userUniqueID != "" {
		state.info.pushID = state.userUniqueID
	}
	if state.info.anchorOnly {
		state.isLive = false
		state.statusKnown = true
	} else if state.hasRoomIdentity() {
		state.isLive, state.statusKnown = parseLiveStatusFromLivePage(body, state.info.roomID)
	} else if state.hasAnchorIdentity {
		state.isLive = false
		state.statusKnown = true
	}
	return state
}

func parseLiveStatusFromLivePage(body, roomID string) (bool, bool) {
	for _, candidate := range livePageStateCandidates(body) {
		if roomObj := roomObjectFromLivePageState(candidate); roomObj != "" {
			statusValue := gjson.Get(roomObj, "status")
			candidateRoomID := firstNonEmptyGJSON(roomObj, "id_str", "id")
			if statusValue.Exists() && (roomID == "" || candidateRoomID == roomID || strings.Contains(candidate, roomID)) {
				return statusValue.Int() == 2, true
			}
		}
		for _, match := range livePageRoomStatusPattern.FindAllStringSubmatch(candidate, -1) {
			if len(match) < 3 {
				continue
			}
			if roomID == "" || match[1] == roomID {
				return match[2] == "2", true
			}
		}
		if roomID != "" {
			idx := strings.Index(candidate, roomID)
			if idx >= 0 {
				end := idx + 1200
				if end > len(candidate) {
					end = len(candidate)
				}
				segment := candidate[idx:end]
				switch {
				case strings.Contains(segment, `"status":2`) || strings.Contains(segment, `\"status\":2`):
					return true, true
				case strings.Contains(segment, `"status":`) || strings.Contains(segment, `\"status\":`):
					return false, true
				}
			}
		}
		if livePageOfflineTextFound(candidate) && (roomID == "" || strings.Contains(candidate, roomID)) {
			return false, true
		}
	}
	return false, false
}

func livePageOfflineTextFound(body string) bool {
	return strings.Contains(body, "直播已结束") ||
		strings.Contains(body, "暂未开播") ||
		strings.Contains(body, "未开播")
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

// fetchLivePageState 从直播页 HTML 中预取 room_id、user_unique_id 和直播状态。
// fetchLivePageState preloads room_id, user_unique_id, and live status from the live page HTML.
func (dl *DouyinLive) fetchLivePageState() error {
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
	pageState := parseLivePageState(body)
	if !pageState.hasKnownPageIdentity() {
		return fmt.Errorf("%w live_id=%s status=%d body_len=%d has_user_unique_id=%t",
			errLivePageStateNotFound,
			dl.liveID,
			resp.GetStatusCode(),
			len(body),
			pageState.userUniqueID != "",
		)
	}
	dl.updateRoomInfoFromLivePage(pageState.info)
	if pageState.statusKnown {
		dl.setLiveStatus(pageState.isLive)
	}
	dl.logger.Debug("从直播间页面预取状态成功", "live_id", dl.liveID, "room_id", pageState.info.roomID, "user_unique_id", pageState.userUniqueID, "live_name", pageState.info.liveName, "title", pageState.info.title)
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
	missingNameSource := "web_enter"
	var livePageErr error

	dl.logger.Debug("开始请求直播间信息", "live_id", dl.liveID)
	if err := dl.fetchLivePageState(); err != nil {
		livePageErr = err
		dl.logger.Debug("从直播间页面预取状态失败，继续请求 web/enter", "live_id", dl.liveID, "err", err)
	}
	err := retry.Do(
		func() error {
			// 核心请求逻辑。
			// Core request flow.
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
		if roomNotFoundErr := dl.roomNotFoundErrorAfterRoomEnter(err, livePageErr); roomNotFoundErr != nil {
			dl.logger.Warn("直播间不存在或页面未返回有效房间状态",
				logFlowArgs("room_info", "room_not_found",
					"live_id", dl.liveID,
					"live_page_err", livePageErr,
					"web_enter_err", err,
				)...,
			)
			return "", roomNotFoundErr
		}
		if fallbackBody, ok := dl.roomEnterFallbackBody(err); ok {
			dl.logger.Debug("web/enter 返回空响应，使用直播间页面状态兜底", "live_id", dl.liveID, "err", err)
			missingNameSource = "live_page_fallback"
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
	if strings.TrimSpace(roomInfo.liveName) == "" {
		dl.logMissingLiveName(missingNameSource, roomInfo)
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
	ctx, cancel := dl.requestContext()
	defer cancel()
	signed, err := dl.signWebcastURL(ctx, "https://live.douyin.com/webcast/room/web/enter/?"+params, dl.initialIMFetchMSToken())
	if err != nil {
		return "", fmt.Errorf("sign web/enter url failed: %w", err)
	}
	url := signed.SignedURL
	roomInfo := dl.roomInfoSnapshot()
	dl.logger.Debug("请求直播间 web/enter",
		logFlowArgs("room_info", "web_enter",
			"live_id", dl.liveID,
			"room_id", roomInfo.roomID,
			"endpoint", "/webcast/room/web/enter/",
			"query_len", len(params),
			"abogus_len", signed.Lengths["a_bogus"],
			"mstoken_len", signed.Lengths["msToken"],
		)...,
	)

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
	if dl.isKnownOfflineStatus() {
		return false
	}
	_, canFallback := dl.roomEnterFallbackBody(err)
	return !canFallback
}

func isRoomInfoEmptyError(err error) bool {
	return errors.Is(err, errRoomInfoEmpty) || (err != nil && strings.Contains(err.Error(), errRoomInfoEmpty.Error()))
}

func (dl *DouyinLive) roomNotFoundErrorAfterRoomEnter(err error, livePageErr error) error {
	if !isRoomInfoEmptyError(err) {
		return nil
	}
	if dl.isKnownOfflineStatus() {
		return nil
	}
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID != "" || strings.TrimSpace(roomInfo.liveName) != "" || strings.TrimSpace(roomInfo.title) != "" || roomInfo.anchorOnly {
		return nil
	}
	if errors.Is(livePageErr, errLivePageStateNotFound) {
		return fmt.Errorf("%w: live_id=%s", ErrRoomNotFound, dl.liveID)
	}
	return nil
}

func (dl *DouyinLive) roomEnterFallbackBody(err error) (string, bool) {
	if !isRoomInfoEmptyError(err) || !dl.isLiveStatus() {
		return "", false
	}
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID == "" || roomInfo.pushID == "" {
		return "", false
	}
	liveName := roomInfo.liveName
	if liveName == dl.liveID {
		liveName = ""
	}
	return fmt.Sprintf(
		`{"status_code":0,"data":{"data":[{"id_str":%q,"status":2,"owner":{"id_str":%q,"nickname":%q},"title":%q}]}}`,
		roomInfo.roomID,
		roomInfo.pushID,
		liveName,
		roomInfo.title,
	), true
}

// logRoomInfoResponseSummary 输出无法解析房间信息时的响应摘要。
// logRoomInfoResponseSummary logs a response summary when room metadata cannot be parsed.
// 参数/Parameters:
//   - body: 无法解析的响应体。 Response body that could not be parsed.
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

	if err := dl.fetchLivePageState(); err == nil {
		roomInfo := dl.roomInfoSnapshot()
		if isLive, known := dl.liveStatusSnapshot(); known {
			if roomInfo.roomID != "" || strings.TrimSpace(roomInfo.liveName) != "" || strings.TrimSpace(roomInfo.title) != "" || roomInfo.anchorOnly {
				step := "live_page_offline"
				msg := "?????????????????"
				if roomInfo.anchorOnly {
					step = "account_offline_no_room"
					msg = "????????????????????????"
				}
				if isLive {
					step = "live_page_online"
					msg = "??????????"
				}
				dl.logger.Info(msg,
					logFlowArgs("room_info", step,
						"live_id", dl.liveID,
						"room_id", roomInfo.roomID,
						"live_name", roomInfo.liveName,
						"title", roomInfo.title,
						"has_room", !roomInfo.anchorOnly && roomInfo.roomID != "",
						"account_only", roomInfo.anchorOnly,
					)...,
				)
				return isLive, nil
			}
		}
	}

	body, err := dl.refreshRoomEnterData()
	if err != nil {
		if isRoomInfoEmptyError(err) {
			roomInfo := dl.roomInfoSnapshot()
			isLive, known := dl.liveStatusSnapshot()
			if known {
				switch {
				case isLive && roomInfo.roomID != "" && roomInfo.pushID != "":
					return true, nil
				case !isLive && (roomInfo.roomID != "" || strings.TrimSpace(roomInfo.liveName) != "" || strings.TrimSpace(roomInfo.title) != "" || roomInfo.anchorOnly):
					return false, nil
				}
			}
		}
		return false, err
	}

	status := gjson.Get(body, "data.data.0.status").Int()
	return status == 2, nil
}

// IsLive 检查直播间当前是否开播。
// IsLive checks whether the live room is currently live.
func (dl *DouyinLive) IsLive() (bool, error) {
	isLive, err := dl.refreshLiveStatusFromAPI()
	if err != nil {
		dl.clearLiveStatus()
		return false, err
	}
	return isLive, nil
}
