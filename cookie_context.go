package douyinLive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// getCookieParts 组装当前有效 Cookie 的键值片段。
// getCookieParts builds key-value parts for the currently effective cookies.
func (dl *DouyinLive) getCookieParts() []string {
	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		cookies := dl.cookieManager.ParseCookies(configCookie)
		parts := make([]string, 0, len(cookies))
		for _, c := range cookies {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		return parts
	}

	parts := []string{fmt.Sprintf("ttwid=%s", dl.ttwid)}
	names := make([]string, 0, len(dl.additionalCookies))
	for name := range dl.additionalCookies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", name, dl.additionalCookies[name]))
	}
	return parts
}

// getCookieString 返回用于请求头的 Cookie 字符串。
// getCookieString returns the Cookie header string.
func (dl *DouyinLive) getCookieString() string {
	parts := dl.getCookieParts()
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

// cookieValue 按名称读取当前有效 Cookie 值，优先使用用户配置的 Cookie。
// cookieValue reads the effective cookie value by name, preferring user-configured cookies.
// 参数/Parameters:
//   - name: Cookie 名称。 Cookie name.
func (dl *DouyinLive) cookieValue(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || dl.cookieManager == nil {
		return ""
	}

	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		for _, c := range dl.cookieManager.ParseCookies(configCookie) {
			if c.Name == name {
				return c.Value
			}
		}
		return ""
	}

	if name == "ttwid" {
		return dl.ttwid
	}
	return dl.additionalCookies[name]
}

// shouldFetchTTWID 判断是否需要自动请求首页获取 ttwid。
// shouldFetchTTWID reports whether ttwid should be fetched automatically from the homepage.
func (dl *DouyinLive) shouldFetchTTWID() bool {
	return !dl.hasConfiguredCookie()
}

// hasConfiguredCookie 判断用户是否显式配置了 Cookie。
// hasConfiguredCookie reports whether the user explicitly configured a Cookie.
func (dl *DouyinLive) hasConfiguredCookie() bool {
	return dl.cookieManager != nil && strings.TrimSpace(dl.cookieManager.GetDouyinCookie()) != ""
}

// setupCookies 将当前 Cookie 写入请求头。
// setupCookies writes the current cookie string into request headers.
func (dl *DouyinLive) setupCookies() {
	dl.headers.Set("Cookie", dl.getCookieString())
}

// fetchTTWID 请求抖音首页并提取 ttwid 及附加 Cookie。
// fetchTTWID requests the Douyin homepage and extracts ttwid plus extra cookies.
func (dl *DouyinLive) fetchTTWID() error {
	ctx, cancel := dl.requestContext()
	defer cancel()

	resp, err := dl.client.R().
		SetContext(ctx).
		Get("https://live.douyin.com/")
	if err != nil {
		return fmt.Errorf("请求TTWID失败: %w", err)
	}

	// 收集所有 Cookie。
	// Collect all cookies.
	cookies := make(map[string]string)
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}

	// 刷新额外 Cookie，避免旧值残留。
	// Refresh extra cookies to avoid stale values.
	dl.additionalCookies = make(map[string]string)

	// 优先使用 ttwid，如果没有找到则报错。
	// Prefer ttwid and fail when it is missing.
	if ttwid, exists := cookies["ttwid"]; exists {
		dl.ttwid = ttwid
	} else {
		return errors.New("未找到TTWID cookie")
	}

	// 存储其他重要 Cookie。
	// Store the remaining useful cookies.
	for name, value := range cookies {
		if name != "ttwid" {
			dl.additionalCookies[name] = value
		}
	}
	return nil
}
