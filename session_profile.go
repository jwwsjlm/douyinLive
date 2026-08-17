package douyinLive

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"crypto/rand"
	"github.com/jwwsjlm/douyinLive/v2/sign"
	"github.com/jwwsjlm/req/v3"
	"math/big"
	"net/http"
)

// sessionProfile 统一持有一次直播会话使用的浏览器画像和相关资源。
// sessionProfile owns the browser identity and resources used by one live session.
type sessionProfile struct {
	ttwid               string
	msToken             string
	userAgent           string
	signer              websocketSigner
	client              *req.Client
	headers             http.Header
	lastUserAgentChange time.Time
	additionalCookies   map[string]string
	cookieManager       *sign.CookieManager
}

// newSessionProfile 创建 UA、Cookie、HTTP 客户端和签名器保持一致的会话画像。
// newSessionProfile creates a session profile with consistent UA, cookies, HTTP client, and signer.
func newSessionProfile(userAgent string, signer websocketSigner, cookie string) sessionProfile {
	if signer == nil {
		signer = newLocalWebsocketSigner()
	}
	signer.UpdateUserAgent(userAgent)
	cookieManager := sign.NewCookieManager()
	if cookie != "" {
		cookieManager.SetDouyinCookie(cookie)
	}
	return sessionProfile{
		userAgent:           userAgent,
		signer:              signer,
		client:              newHTTPClient(userAgent),
		headers:             make(http.Header),
		lastUserAgentChange: time.Now(),
		additionalCookies:   make(map[string]string),
		cookieManager:       cookieManager,
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

var impersonatedUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36",
}

var userAgentSelector struct {
	sync.Mutex
	order  []int
	cursor int
}

// newHTTPUserAgent 随机选择一个用于 HTTP 伪装的浏览器 UA。
// newHTTPUserAgent randomly selects a browser user agent for HTTP impersonation.
func newHTTPUserAgent() string {
	return newHTTPUserAgentExcept("")
}

// newHTTPUserAgentExcept 随机选择 UA，并在存在其他候选项时避免继续使用旧值。
// newHTTPUserAgentExcept selects a random UA and avoids the previous value when alternatives exist.
func newHTTPUserAgentExcept(excluded string) string {
	userAgentSelector.Lock()
	defer userAgentSelector.Unlock()

	if len(impersonatedUserAgents) == 0 {
		return ""
	}

	for range len(impersonatedUserAgents) {
		if userAgentSelector.cursor >= len(userAgentSelector.order) {
			userAgentSelector.order = shuffledUserAgentIndexes(len(impersonatedUserAgents))
			userAgentSelector.cursor = 0
		}
		index := userAgentSelector.order[userAgentSelector.cursor]
		userAgentSelector.cursor++
		candidate := impersonatedUserAgents[index]
		if candidate != excluded || len(impersonatedUserAgents) == 1 {
			return candidate
		}
	}

	return impersonatedUserAgents[0]
}

// shuffledUserAgentIndexes 为一个选择周期生成随机且不重复的 UA 顺序。
// shuffledUserAgentIndexes creates a randomized, non-repeating UA order for one selection cycle.
func shuffledUserAgentIndexes(count int) []int {
	order := make([]int, count)
	for index := range order {
		order[index] = index
	}
	for index := count - 1; index > 0; index-- {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			continue
		}
		swapIndex := int(randomIndex.Int64())
		order[index], order[swapIndex] = order[swapIndex], order[index]
	}
	return order
}

// newHTTPClient 创建带浏览器伪装和超时设置的 HTTP 客户端。
// newHTTPClient creates an HTTP client with browser impersonation and timeout settings.
// 参数/Parameters:
//   - userAgent: 请求使用的浏览器 User-Agent。 Browser User-Agent used for requests.
func newHTTPClient(userAgent string) *req.Client {
	return req.C().
		ImpersonateChromeWithOS(req.BrowserOSWindows).
		EnableHTTP3().
		EnableHTTP3FallbackOnError().
		SetUserAgent(userAgent).
		SetTimeout(httpRequestTimeout)
}

// rebuildHTTPClientAndHeaders 重建 HTTP 客户端并刷新基础请求头。
// rebuildHTTPClientAndHeaders rebuilds the HTTP client and refreshes base headers.
func (dl *DouyinLive) rebuildHTTPClientAndHeaders() {
	oldClient := dl.client
	dl.client = newHTTPClient(dl.userAgent)
	dl.headers = make(http.Header)
	dl.headers.Set("User-Agent", dl.userAgent)
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

func browserClientHintHeaders(userAgent string) map[string]string {
	major := chromeMajorVersionFromUserAgent(userAgent)
	return map[string]string{
		"sec-ch-ua":          fmt.Sprintf(`"Not;A=Brand";v="8", "Chromium";v="%s", "Google Chrome";v="%s"`, major, major),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
}
