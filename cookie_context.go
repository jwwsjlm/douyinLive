package douyinLive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// getCookieParts 组装当前有效 Cookie 的键值片段。
// getCookieParts builds key-value parts for the currently effective cookies.
// 无论走用户配置还是自动 ttwid 分支，都会补齐 PC 客户端固定携带的 Cookie，
// 使请求与桌面客户端保持同一指纹；已存在的同名 Cookie 以现有值为准。
// Either branch is completed with the fixed PC-client cookies so requests carry the same
// fingerprint as the desktop client; existing names keep their current value.
func (dl *DouyinLive) getCookieParts() []string {
	present := make(map[string]struct{})
	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		cookies := dl.cookieManager.ParseCookies(configCookie)
		parts := make([]string, 0, len(cookies)+len(pcClientCookieSeeds))
		for _, c := range cookies {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
			present[c.Name] = struct{}{}
		}
		return append(parts, dl.protocol.cookieParts(present)...)
	}

	parts := []string{fmt.Sprintf("ttwid=%s", dl.ttwid)}
	present["ttwid"] = struct{}{}
	names := make([]string, 0, len(dl.additionalCookies))
	for name := range dl.additionalCookies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", name, dl.additionalCookies[name]))
		present[name] = struct{}{}
	}
	return append(parts, dl.protocol.cookieParts(present)...)
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

// shouldFetchTTWID 判断当前会话是否还需要获取匿名 ttwid。
// shouldFetchTTWID reports whether this session still needs an anonymous ttwid.
func (dl *DouyinLive) shouldFetchTTWID() bool {
	return !dl.hasConfiguredCookie() && strings.TrimSpace(dl.ttwid) == ""
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

// fetchTTWID 优先注册匿名 ttwid，失败时回退直播首页。
// fetchTTWID registers an anonymous ttwid, falling back to the live homepage.
func (dl *DouyinLive) fetchTTWID(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	registrationCtx, cancel := context.WithTimeout(ctx, ttwidRegistrationTimeout)
	resp, err := dl.client.R().
		SetContext(registrationCtx).
		SetHeader("Content-Type", "application/json").
		SetBodyString(`{"region":"cn","aid":1768,"needFid":false,"service":"www.ixigua.com","migrate_info":{"ticket":"","source":"node"},"cbUrlProtocol":"https","union":true}`).
		Post("https://ttwid.bytedance.com/ttwid/union/register/")
	cancel()
	if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := dl.storeTTWIDCookies(resp.Cookies()); err == nil {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dl.logger.Debug("轻量TTWID获取未成功，回退直播首页", "live_id", dl.liveID)
	resp, err = dl.client.R().
		SetContext(ctx).
		Get("https://live.douyin.com/")
	if err != nil {
		return fmt.Errorf("请求TTWID失败: %w", err)
	}

	if resp == nil {
		return errors.New("获取TTWID响应为空")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("获取TTWID响应异常 status=%d", resp.StatusCode)
	}
	return dl.storeTTWIDCookies(resp.Cookies())
}

func (dl *DouyinLive) storeTTWIDCookies(responseCookies []*http.Cookie) error {
	cookies := make(map[string]string)
	for _, c := range responseCookies {
		cookies[c.Name] = c.Value
	}
	if strings.TrimSpace(cookies["ttwid"]) == "" {
		return errors.New("未找到TTWID cookie")
	}
	dl.ttwid = cookies["ttwid"]
	delete(cookies, "ttwid")
	dl.additionalCookies = cookies
	return nil
}
