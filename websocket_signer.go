package douyinLive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	tikhub "github.com/jwwsjlm/Tikhub"
	"github.com/jwwsjlm/douyinLive/v2/jsScript"
	"github.com/jwwsjlm/douyinLive/v2/utils"
)

const (
	SignProviderLocal  = "local"
	SignProviderTikHub = "tikhub"
)

var (
	ErrTikHubTokenEmpty  = errors.New("tikhub token 未配置")
	ErrTikHubSignInvalid = errors.New("tikhub 签名响应无效")
)

type websocketSigner interface {
	Name() string
	Sign(ctx context.Context, roomID, userUniqueID, userAgent string) (string, error)
	UpdateUserAgent(userAgent string)
}

type localWebsocketSigner struct{}

func newLocalWebsocketSigner() websocketSigner {
	return localWebsocketSigner{}
}

func (localWebsocketSigner) Name() string {
	return SignProviderLocal
}

func (localWebsocketSigner) Sign(_ context.Context, roomID, userUniqueID, _ string) (string, error) {
	return jsScript.ExecuteJS(utils.GetxMSStub(
		utils.NewOrderedMap(roomID, userUniqueID),
	)), nil
}

func (localWebsocketSigner) UpdateUserAgent(string) {}

type tikhubWebsocketSigner struct {
	token  string
	client *tikhub.Client
}

func newTikHubWebsocketSigner(token, userAgent string) websocketSigner {
	token = strings.TrimSpace(token)
	return &tikhubWebsocketSigner{
		token:  token,
		client: newTikHubClient(token, userAgent),
	}
}

func (s *tikhubWebsocketSigner) Name() string {
	return SignProviderTikHub
}

func (s *tikhubWebsocketSigner) LogStatus(logger logSink, liveID string) {
	if logger == nil {
		return
	}
	token := strings.TrimSpace(s.token)
	if token == "" {
		logger.Warn("TikHub API Key 未配置", "live_id", liveID)
		return
	}

	hash := sha256.Sum256([]byte(token))
	logger.Info(
		"TikHub API Key 已加载",
		"live_id", liveID,
		"key_len", len(token),
		"key_mask", maskSecret(token),
		"key_sha256_8", hex.EncodeToString(hash[:])[:8],
	)
}

func (s *tikhubWebsocketSigner) UpdateUserAgent(userAgent string) {
	if s.client == nil {
		s.client = newTikHubClient(s.token, userAgent)
		return
	}
	client := s.client.ReqClient()
	if client != nil {
		client.SetUserAgent(userAgent)
	}
}

func (s *tikhubWebsocketSigner) Sign(ctx context.Context, roomID, userUniqueID, userAgent string) (string, error) {
	token := strings.TrimSpace(s.token)
	if token == "" {
		return "", ErrTikHubTokenEmpty
	}
	if roomID == "" || userUniqueID == "" {
		return "", fmt.Errorf("%w: room_id 或 user_unique_id 为空", ErrTikHubSignInvalid)
	}

	if s.client == nil {
		s.client = newTikHubClient(token, userAgent)
	}
	resp, err := s.client.DouyinWeb.GenerateWssXbSignature(ctx, tikhub.DouyinWebGenerateWssXbSignatureRequest{
		UserAgent:    userAgent,
		RoomID:       roomID,
		UserUniqueID: userUniqueID,
	})
	if err != nil {
		return "", fmt.Errorf("请求 tikhub 签名失败: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("%w: nil response", ErrTikHubSignInvalid)
	}
	if resp.StatusCode != 0 && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices) {
		return "", fmt.Errorf("tikhub 签名接口返回异常 status=%d body=%s", resp.StatusCode, string(resp.Raw))
	}
	if !isTikHubSuccessCode(resp.Code) {
		return "", fmt.Errorf("tikhub 签名接口业务失败 code=%d message=%s body=%s", resp.Code, firstNonEmptyString(resp.MessageZH, resp.Message), string(resp.Raw))
	}

	signature := extractTikHubSignature(resp.Raw)
	if signature == "" && len(resp.Data) > 0 {
		signature = extractTikHubSignature(resp.Data)
	}
	if signature == "" {
		return "", fmt.Errorf("%w: body=%s", ErrTikHubSignInvalid, string(resp.Raw))
	}
	signature = normalizeTikHubSignature(signature)
	if signature == "" {
		return "", fmt.Errorf("%w: body=%s", ErrTikHubSignInvalid, string(resp.Raw))
	}
	return signature, nil
}

func isTikHubSuccessCode(code int) bool {
	return code == 0 || code == http.StatusOK
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return ""
	case len(value) <= 8:
		return strings.Repeat("*", len(value))
	default:
		return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func newTikHubClient(token, userAgent string) *tikhub.Client {
	return tikhub.NewClient(strings.TrimSpace(token),
		tikhub.WithTimeout(httpRequestTimeout),
		tikhub.WithUserAgent(userAgent),
	)
}

func extractTikHubSignature(body []byte) string {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(findSignatureValue(payload, true))
}

func findSignatureValue(value interface{}, allowRawString bool) string {
	switch v := value.(type) {
	case string:
		if allowRawString && isLikelySignature(v) {
			return v
		}
	case map[string]interface{}:
		for _, key := range []string{
			"data",
			"xb",
			"x-bogus",
			"X-Bogus",
			"x_bogus",
			"X_Bogus",
			"xBogus",
			"XBogus",
			"signature",
			"sign",
			"result",
			"value",
			"wss_signature",
		} {
			if candidate, ok := v[key]; ok {
				if signature := findSignatureValue(candidate, true); signature != "" {
					return signature
				}
			}
		}
		for _, candidate := range v {
			if signature := findSignatureValue(candidate, false); signature != "" {
				return signature
			}
		}
	case []interface{}:
		for _, candidate := range v {
			if signature := findSignatureValue(candidate, false); signature != "" {
				return signature
			}
		}
	}
	return ""
}

func isLikelySignature(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "request successful") || strings.Contains(lower, "请求成功") {
		return false
	}
	return true
}

func normalizeTikHubSignature(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if parsed, err := url.Parse(value); err == nil && parsed.RawQuery != "" {
		if signature := signatureFromValues(parsed.Query()); signature != "" {
			return signature
		}
	}
	if strings.Contains(value, "=") {
		queryText := strings.TrimPrefix(value, "?")
		if values, err := url.ParseQuery(queryText); err == nil {
			if signature := signatureFromValues(values); signature != "" {
				return signature
			}
		}
	}
	if strings.Contains(value, "%") {
		if decoded, err := url.PathUnescape(value); err == nil {
			return strings.TrimSpace(decoded)
		}
	}
	return value
}

func signatureFromValues(values url.Values) string {
	for _, key := range []string{
		"signature",
		"xb",
		"X-Bogus",
		"x-bogus",
		"x_bogus",
		"X_Bogus",
		"xBogus",
		"XBogus",
		"wss_signature",
		"sign",
		"result",
		"value",
	} {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}
