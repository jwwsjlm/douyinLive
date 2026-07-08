package douyinLive

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"github.com/jwwsjlm/douyinLive/v2/utils"
)

//go:embed jsScript/bdms.js
var bdmsJS string

//go:embed jsScript/bdms_env.js
var bdmsEnvJS string

//go:embed jsScript/bdms_sign_url.js
var bdmsSignURLJS string

// BDMSURLSignResult 表示 BDMS 本地签名后的 webcast URL 以及安全诊断信息。
type BDMSURLSignResult struct {
	SignedURL         string         `json:"signedUrl"`
	SignedURLRedacted string         `json:"signedUrlRedacted"`
	Lengths           map[string]int `json:"lengths"`
}

// signWebcastURL 使用 Goja 运行内嵌 bdms.js，为 /webcast/* URL 生成 msToken 与 a_bogus。
func (dl *DouyinLive) signWebcastURL(ctx context.Context, unsignedURL string, msToken string) (*BDMSURLSignResult, error) {
	if dl == nil {
		return nil, errors.New("nil DouyinLive")
	}
	return signURLWithLocalBDMS(ctx, unsignedURL, dl.getCookieString(), msToken, dl.userAgent)
}

func signURLWithLocalBDMS(ctx context.Context, unsignedURL string, cookie string, msToken string, userAgent string) (*BDMSURLSignResult, error) {
	unsignedURL = strings.TrimSpace(unsignedURL)
	if unsignedURL == "" {
		return nil, errors.New("unsigned url is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	hasProvidedMsToken := urlHasQueryKey(unsignedURL, "msToken")
	externalMsToken := firstNonEmptyBDMSString(strings.TrimSpace(msToken), pickCookieValueForBDMS(cookie, "msToken"))
	canRegenerateMsToken := !hasProvidedMsToken && externalMsToken == ""
	maxAttempts := 1
	if canRegenerateMsToken {
		maxAttempts = 12
	}

	lastSignedURL := unsignedURL
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		candidateURL := ensureBDMSMsTokenInURL(unsignedURL, cookie, externalMsToken)
		signedURL, err := signURLWithGojaBDMS(candidateURL, cookie, userAgent)
		if err != nil {
			lastErr = err
			continue
		}
		lastSignedURL = signedURL
		if !canRegenerateMsToken || queryValueLength(lastSignedURL, "a_bogus") == 188 {
			break
		}
	}
	if lastSignedURL == "" || lastSignedURL == unsignedURL && lastErr != nil {
		return nil, lastErr
	}

	result := &BDMSURLSignResult{
		SignedURL:         lastSignedURL,
		SignedURLRedacted: redactSignedURLForLog(lastSignedURL),
		Lengths:           queryParamLengths(lastSignedURL, "msToken", "a_bogus", "X-Bogus", "_signature"),
	}
	if result.SignedURL == "" {
		return nil, errors.New("bdms signer returned empty signed url")
	}
	return result, nil
}

func signURLWithGojaBDMS(unsignedURL string, cookie string, userAgent string) (string, error) {
	vm := goja.New()
	if err := installGojaBDMSEnvironment(vm, cookie, userAgent); err != nil {
		return "", err
	}
	if _, err := vm.RunString(bdmsJS); err != nil {
		return "", fmt.Errorf("load bdms.js into goja failed: %w", err)
	}
	if _, err := vm.RunString(bdmsSignURLJS); err != nil {
		return "", fmt.Errorf("load bdms sign helper into goja failed: %w", err)
	}
	signURL, ok := goja.AssertFunction(vm.Get("__signBDMSURL"))
	if !ok {
		return "", errors.New("__signBDMSURL is not available")
	}
	value, err := signURL(goja.Undefined(), vm.ToValue(unsignedURL))
	if err != nil {
		return "", fmt.Errorf("execute bdms goja signer failed: %w", err)
	}
	signedURL := strings.TrimSpace(value.String())
	if signedURL == "" {
		return "", errors.New("bdms goja signer returned empty url")
	}
	return signedURL, nil
}

func installGojaBDMSEnvironment(vm *goja.Runtime, cookie string, userAgent string) error {
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	}
	if _, err := vm.RunString(bdmsEnvJS); err != nil {
		return fmt.Errorf("load bdms env into goja failed: %w", err)
	}
	install, ok := goja.AssertFunction(vm.Get("__installBDMSEnvironment"))
	if !ok {
		return errors.New("__installBDMSEnvironment is not available")
	}
	if _, err := install(goja.Undefined(), vm.ToValue(userAgent), vm.ToValue(cookie)); err != nil {
		return fmt.Errorf("install bdms goja environment failed: %w", err)
	}
	return nil
}

func ensureBDMSMsTokenInURL(rawURL string, cookie string, externalMsToken string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("msToken") == "" {
		if externalMsToken == "" {
			externalMsToken = pickCookieValueForBDMS(cookie, "msToken")
		}
		if externalMsToken == "" {
			externalMsToken = utils.GenerateMsToken(172)
		}
		q.Set("msToken", externalMsToken)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func urlHasQueryKey(rawURL string, key string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, key+"=")
	}
	_, ok := u.Query()[key]
	return ok
}

func pickCookieValueForBDMS(cookie string, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		idx := strings.IndexByte(part, '=')
		if idx <= 0 {
			continue
		}
		if strings.TrimSpace(part[:idx]) != name {
			continue
		}
		value, err := url.QueryUnescape(strings.TrimSpace(part[idx+1:]))
		if err != nil {
			return strings.TrimSpace(part[idx+1:])
		}
		return value
	}
	return ""
}

func firstNonEmptyBDMSString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func queryParamLengths(rawURL string, keys ...string) map[string]int {
	lengths := map[string]int{}
	u, err := url.Parse(rawURL)
	if err != nil {
		return lengths
	}
	q := u.Query()
	for _, key := range keys {
		if value := q.Get(key); value != "" {
			lengths[key] = len(value)
		}
	}
	return lengths
}

func queryValueLength(rawURL string, key string) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	return len(u.Query().Get(key))
}

func redactSignedURLForLog(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		replacer := rawURL
		for _, key := range []string{"msToken", "a_bogus", "X-Bogus", "_signature"} {
			replacer = redactQueryValue(replacer, key)
		}
		return replacer
	}
	q := u.Query()
	for _, key := range []string{"msToken", "a_bogus", "X-Bogus", "_signature"} {
		if values, ok := q[key]; ok && len(values) > 0 {
			q.Set(key, fmt.Sprintf("<redacted:%d>", len(values[0])))
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func redactQueryValue(rawURL string, key string) string {
	marker := key + "="
	idx := strings.Index(rawURL, marker)
	if idx < 0 {
		return rawURL
	}
	start := idx + len(marker)
	end := strings.IndexByte(rawURL[start:], '&')
	if end < 0 {
		end = len(rawURL)
	} else {
		end += start
	}
	return rawURL[:start] + "<redacted>" + rawURL[end:]
}
