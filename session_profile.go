package douyinLive

import (
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/sign"
	"github.com/jwwsjlm/req/v3"
)

// sessionProfile 统一持有一次直播会话使用的浏览器画像和相关资源。
// sessionProfile owns the browser identity and resources used by one live session.
type sessionProfile struct {
	ttwid               string
	msToken             string
	userAgent           string
	signer              websocketSigner
	client              *req.Client
	proxy               proxyPolicy
	headers             http.Header
	lastUserAgentChange time.Time
	additionalCookies   map[string]string
	cookieManager       *sign.CookieManager
	fingerprint         browserFingerprint
	protocol            ProtocolMode
	deviceIdentity      pcClientDeviceIdentity
}

// newSessionProfile 创建 UA、Cookie、HTTP 客户端和签名器保持一致的会话画像。
// newSessionProfile creates a session profile with consistent UA, cookies, HTTP client, and signer.
// 参数/Parameters:
//   - protocol: 该会话使用的协议画像。 Protocol profile used by this session.
//   - userAgent: 已按画像选定的 User-Agent。 User agent already selected for the profile.
func newSessionProfile(protocol ProtocolMode, userAgent string, signer websocketSigner, cookie string, proxy proxyPolicy) sessionProfile {
	if signer == nil {
		signer = newLocalWebsocketSigner()
	}
	signer.UpdateUserAgent(userAgent)
	fingerprint := newBrowserFingerprint()
	if updater, ok := signer.(websocketSignerFingerprintUpdater); ok {
		updater.UpdateBrowserFingerprint(fingerprint)
	}
	cookieManager := sign.NewCookieManager()
	if cookie != "" {
		cookieManager.SetDouyinCookie(cookie)
	}
	return sessionProfile{
		userAgent:           userAgent,
		signer:              signer,
		client:              newHTTPClient(userAgent, proxy),
		proxy:               proxy,
		headers:             make(http.Header),
		lastUserAgentChange: time.Now(),
		additionalCookies:   make(map[string]string),
		cookieManager:       cookieManager,
		fingerprint:         fingerprint,
		protocol:            protocol,
		deviceIdentity:      newPCClientDeviceIdentity(fingerprint.ID),
	}
}

// close 释放会话画像持有的签名运行时和 HTTP 空闲连接。
// close releases the signer runtime and idle HTTP connections owned by the profile.
func (p *sessionProfile) close() {
	if p == nil {
		return
	}
	if closer, ok := p.signer.(websocketSignerCloser); ok {
		closer.Close()
	}
	closeHTTPClientIdleConnections(p.client)
}

const httpImpersonationChromeMajor = "133"

// selectUserAgent 从候选池中选择 UA，并在存在其他候选项时避免继续使用旧值。
// selectUserAgent picks a UA and avoids the previous value when alternatives exist.
// 参数/Parameters:
//   - candidates: UA 候选池。 User agent candidate pool.
//   - excluded: 希望避免的旧值；无其他候选时仍会返回它。 Previously used value to avoid when possible.
func selectUserAgent(candidates []string, excluded string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	start := rand.IntN(len(candidates))
	for offset := range len(candidates) {
		candidate := candidates[(start+offset)%len(candidates)]
		if candidate != excluded {
			return candidate
		}
	}
	return candidates[start]
}

// newHTTPClient 创建带浏览器伪装和超时设置的 HTTP 客户端。
// newHTTPClient creates an HTTP client with browser impersonation and timeout settings.
// 参数/Parameters:
//   - userAgent: 请求使用的浏览器 User-Agent。 Browser User-Agent used for requests.
func newHTTPClient(userAgent string, proxy proxyPolicy) *req.Client {
	client := req.C().
		ImpersonateChromeWithOS(req.BrowserOSWindows).
		SetProxy(proxy.resolve).
		SetUserAgent(userAgent).
		SetTimeout(httpRequestTimeout)
	if proxy.disableHTTP3 {
		client.DisableHTTP3()
	} else {
		client.EnableHTTP3().EnableHTTP3FallbackOnError()
	}
	return client
}

// rebuildHTTPClientAndHeaders 重建 HTTP 客户端并刷新基础请求头。
// rebuildHTTPClientAndHeaders rebuilds the HTTP client and refreshes base headers.
func (dl *DouyinLive) rebuildHTTPClientAndHeaders() {
	oldClient := dl.client
	dl.client = newHTTPClient(dl.userAgent, dl.proxy)
	dl.headers = make(http.Header)
	dl.headers.Set("User-Agent", dl.userAgent)
	// 头集合被重建后必须重新注入协议画像头，否则 PC 专有指纹会丢失。
	// Profile headers must be re-injected after the header set is rebuilt.
	dl.applyProtocolHeadersToHTTPHeader(dl.headers)
	dl.refreshSignerUserAgent()
	closeHTTPClientIdleConnections(oldClient)
}

// closeHTTPClientIdleConnections 关闭 req 客户端保留的 HTTP/1.1、HTTP/2 和 HTTP/3 空闲连接。
// closeHTTPClientIdleConnections closes idle HTTP/1.1, HTTP/2, and HTTP/3 connections retained by req.
func closeHTTPClientIdleConnections(client *req.Client) {
	if client == nil {
		return
	}
	if transport := client.GetTransport(); transport != nil {
		transport.CloseIdleConnections()
	}
}

// refreshSignerUserAgent 将当前 UA 同步给签名器。
// refreshSignerUserAgent syncs the current user agent to the signer.
func (dl *DouyinLive) refreshSignerUserAgent() {
	if dl.signer != nil {
		dl.signer.UpdateUserAgent(dl.userAgent)
	}
}

// refreshSignerFingerprint 将当前会话画像同步给本地签名器。
// refreshSignerFingerprint syncs the current browser fingerprint to the local signer.
func (dl *DouyinLive) refreshSignerFingerprint() {
	if updater, ok := dl.signer.(websocketSignerFingerprintUpdater); ok {
		updater.UpdateBrowserFingerprint(dl.fingerprint)
	}
}

// chromeVersionFromUserAgent 从 User-Agent 中提取 Chrome 完整版本号。
// chromeVersionFromUserAgent extracts the full Chrome version from User-Agent.
// 参数/Parameters:
//   - userAgent: 浏览器 User-Agent 字符串。 Browser User-Agent string.
func chromeVersionFromUserAgent(userAgent string) string {
	const marker = "Chrome/"
	if idx := strings.Index(userAgent, marker); idx >= 0 {
		version := userAgent[idx+len(marker):]
		if end := strings.IndexByte(version, ' '); end >= 0 {
			version = version[:end]
		}
		if version != "" {
			return version
		}
	}
	return browserVersionFromUserAgent(userAgent)
}

func chromeMajorVersionFromUserAgent(userAgent string) string {
	version := chromeVersionFromUserAgent(userAgent)
	if major, _, ok := strings.Cut(version, "."); ok && major != "" {
		return major
	}
	if version != "" {
		return version
	}
	return "133"
}

// eachProtocolHeader 遍历当前协议画像的专有请求头。
// eachProtocolHeader visits the profile-specific request headers.
func (dl *DouyinLive) eachProtocolHeader(visit func(string, string)) {
	if visit == nil {
		return
	}
	for key, value := range dl.protocol.clientHints() {
		visit(key, value)
	}
	for key, value := range dl.protocol.requestHeaders(dl.deviceIdentity) {
		visit(key, value)
	}
}

// applyProtocolHeaders 把协议画像相关的请求头写入 map 形式的请求头集合。
// applyProtocolHeaders writes protocol-profile headers into a map-based header set.
func (dl *DouyinLive) applyProtocolHeaders(headers map[string]string) {
	if headers == nil {
		return
	}
	dl.eachProtocolHeader(func(key, value string) { headers[key] = value })
}

// applyProtocolHeadersToHTTPHeader 把协议画像相关的请求头写入 http.Header。
// applyProtocolHeadersToHTTPHeader writes protocol-profile request headers into an http.Header.
// 参数/Parameters:
//   - headers: 目标请求头集合。 Target header set.
func (dl *DouyinLive) applyProtocolHeadersToHTTPHeader(headers http.Header) {
	if headers == nil {
		return
	}
	dl.eachProtocolHeader(headers.Set)
}
