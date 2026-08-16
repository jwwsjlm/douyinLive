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

// 将大体积脚本在进程启动时预编译，避免首个直播间请求重复解析。
// Precompile embedded scripts at process startup to avoid reparsing them for the first live-room request.
var (
	bdmsEnvProgram     = goja.MustCompile("bdms_env.js", bdmsEnvJS, false)
	bdmsProgram        = goja.MustCompile("bdms.js", bdmsJS, false)
	bdmsSignURLProgram = goja.MustCompile("bdms_sign_url.js", bdmsSignURLJS, false)
)

// BDMSURLSignResult 表示 BDMS 本地签名后的 webcast URL 以及安全诊断信息。
type BDMSURLSignResult struct {
	SignedURL         string         `json:"signedUrl"`
	SignedURLRedacted string         `json:"signedUrlRedacted"`
	Lengths           map[string]int `json:"lengths"`
}

// localBDMSRuntime 保存一个已加载 bdms.js 的 Goja 运行时。
// localBDMSRuntime keeps a Goja runtime with bdms.js already loaded.
type localBDMSRuntime struct {
	cookie    string
	userAgent string
	vm        *goja.Runtime
	signURL   func(goja.Value, ...goja.Value) (goja.Value, error)
}

// signWebcastURL 使用 Goja 运行内嵌 bdms.js，为 /webcast/* URL 生成 msToken 与 a_bogus。
func (dl *DouyinLive) signWebcastURL(ctx context.Context, unsignedURL string, msToken string) (*BDMSURLSignResult, error) {
	if dl == nil {
		return nil, errors.New("nil DouyinLive")
	}

	cookie := dl.getCookieString()
	dl.bdmsMu.Lock()
	defer dl.bdmsMu.Unlock()
	return signURLWithBDMS(ctx, unsignedURL, cookie, msToken, dl.userAgent, dl.signURLWithCachedGojaBDMSLocked)
}

// signURLWithLocalBDMS 使用临时 Goja 运行时签名，供独立调用与测试使用。
// signURLWithLocalBDMS signs with a temporary Goja runtime for standalone callers and tests.
func signURLWithLocalBDMS(ctx context.Context, unsignedURL string, cookie string, msToken string, userAgent string) (*BDMSURLSignResult, error) {
	return signURLWithBDMS(ctx, unsignedURL, cookie, msToken, userAgent, signURLWithGojaBDMS)
}

type bdmsURLSigner func(unsignedURL string, cookie string, userAgent string) (string, error)

// signURLWithBDMS 执行 URL 参数补齐、签名及结果校验。
// signURLWithBDMS completes URL parameters, signs the URL, and validates the result.
func signURLWithBDMS(ctx context.Context, unsignedURL string, cookie string, msToken string, userAgent string, signer bdmsURLSigner) (*BDMSURLSignResult, error) {
	unsignedURL = strings.TrimSpace(unsignedURL)
	if unsignedURL == "" {
		return nil, errors.New("unsigned url is empty")
	}
	if signer == nil {
		return nil, errors.New("bdms signer is nil")
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
		signedURL, err := signer(candidateURL, cookie, userAgent)
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

// signURLWithCachedGojaBDMSLocked 复用与当前 Cookie、UA 对应的 BDMS 运行时。
// signURLWithCachedGojaBDMSLocked reuses the BDMS runtime matching the current cookie and user agent.
// 调用方必须持有 dl.bdmsMu。
// The caller must hold dl.bdmsMu.
func (dl *DouyinLive) signURLWithCachedGojaBDMSLocked(unsignedURL string, cookie string, userAgent string) (string, error) {
	runtime := dl.bdmsRuntime
	if runtime == nil || runtime.cookie != cookie || runtime.userAgent != userAgent {
		var err error
		runtime, err = newLocalBDMSRuntime(cookie, userAgent)
		if err != nil {
			return "", err
		}
		dl.bdmsRuntime = runtime
	}
	return runtime.sign(unsignedURL)
}

func signURLWithGojaBDMS(unsignedURL string, cookie string, userAgent string) (string, error) {
	runtime, err := newLocalBDMSRuntime(cookie, userAgent)
	if err != nil {
		return "", err
	}
	return runtime.sign(unsignedURL)
}

// newLocalBDMSRuntime 创建并预加载一个 BDMS Goja 运行时。
// newLocalBDMSRuntime creates and preloads a BDMS Goja runtime.
func newLocalBDMSRuntime(cookie string, userAgent string) (*localBDMSRuntime, error) {
	vm := goja.New()
	if err := installGojaBDMSEnvironment(vm, cookie, userAgent); err != nil {
		return nil, err
	}
	if _, err := vm.RunProgram(bdmsProgram); err != nil {
		return nil, fmt.Errorf("load bdms.js into goja failed: %w", err)
	}
	if _, err := vm.RunProgram(bdmsSignURLProgram); err != nil {
		return nil, fmt.Errorf("load bdms sign helper into goja failed: %w", err)
	}
	signURL, ok := goja.AssertFunction(vm.Get("__signBDMSURL"))
	if !ok {
		return nil, errors.New("__signBDMSURL is not available")
	}
	return &localBDMSRuntime{
		cookie:    cookie,
		userAgent: userAgent,
		vm:        vm,
		signURL:   signURL,
	}, nil
}

// sign 在已加载的 BDMS 运行时中对 URL 签名。
// sign signs a URL in an already loaded BDMS runtime.
func (runtime *localBDMSRuntime) sign(unsignedURL string) (string, error) {
	if runtime == nil || runtime.vm == nil || runtime.signURL == nil {
		return "", errors.New("bdms runtime is not initialized")
	}
	value, err := runtime.signURL(goja.Undefined(), runtime.vm.ToValue(unsignedURL))
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
	if _, err := vm.RunProgram(bdmsEnvProgram); err != nil {
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
