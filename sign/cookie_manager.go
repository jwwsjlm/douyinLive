package sign

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// cookieConfig 定义 CookieManager 内部使用的 YAML 配置结构。
// cookieConfig defines the YAML shape used internally by CookieManager.
type cookieConfig struct {
	Cookie struct {
		Douyin string `yaml:"douyin"`
	} `yaml:"cookie"`
}

// CookieManager 管理抖音 Cookie 配置和请求用 Cookie jar。
// CookieManager manages Douyin cookie configuration and the request cookie jar.
type CookieManager struct {
	config *cookieConfig
	jar    *cookiejar.Jar
}

// NewCookieManager 创建 Cookie 管理器。
// NewCookieManager creates a cookie manager.
func NewCookieManager() *CookieManager {
	jar, _ := cookiejar.New(nil)
	return &CookieManager{
		jar: jar,
	}
}

// LoadConfig 从 YAML 文件加载 Cookie 配置。
// LoadConfig loads cookie configuration from a YAML file.
// 参数/Parameters:
//   - path: YAML 配置文件路径。 YAML config file path.
func (cm *CookieManager) LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config cookieConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	cm.config = &config
	return nil
}

// LoadFromEnv 从环境变量加载 Cookie。
// LoadFromEnv loads cookies from environment variables.
func (cm *CookieManager) LoadFromEnv() {
	if cm.config == nil {
		cm.config = &cookieConfig{}
	}

	cm.config.Cookie.Douyin = os.Getenv("DOUYIN_COOKIE")
}

// GetDouyinCookie 获取当前抖音 Cookie。
// GetDouyinCookie returns the current Douyin cookie.
func (cm *CookieManager) GetDouyinCookie() string {
	if cm.config != nil {
		return cm.config.Cookie.Douyin
	}
	return ""
}

// SetDouyinCookie 手动设置抖音 Cookie。
// SetDouyinCookie sets the Douyin cookie manually.
// 参数/Parameters:
//   - cookie: 抖音 Cookie 字符串。 Douyin cookie string.
func (cm *CookieManager) SetDouyinCookie(cookie string) {
	if cm.config == nil {
		cm.config = &cookieConfig{}
	}
	cm.config.Cookie.Douyin = cookie
}

// ParseCookies 将 Cookie 字符串解析为 http.Cookie 列表。
// ParseCookies parses a cookie header string into http.Cookie values.
// 参数/Parameters:
//   - cookieStr: Cookie 请求头字符串。 Cookie header string.
func (cm *CookieManager) ParseCookies(cookieStr string) []*http.Cookie {
	var cookies []*http.Cookie
	if cookieStr == "" {
		return cookies
	}

	pairs := strings.Split(cookieStr, ";")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			continue
		}
		cookies = append(cookies, &http.Cookie{
			Name:  strings.TrimSpace(parts[0]),
			Value: strings.TrimSpace(parts[1]),
		})
	}

	return cookies
}

// SetCookies 将 Cookie 字符串写入 jar。
// SetCookies stores a cookie string in the jar for the given URL.
// 参数/Parameters:
//   - rawURL: Cookie 归属的 URL。 URL that owns the cookies.
//   - cookieStr: Cookie 请求头字符串。 Cookie header string.
func (cm *CookieManager) SetCookies(rawURL string, cookieStr string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	cookies := cm.ParseCookies(cookieStr)
	cm.jar.SetCookies(parsedURL, cookies)
	return nil
}

// GetCookies 从 jar 中读取指定 URL 的 Cookie。
// GetCookies returns cookies from the jar for the given URL.
// 参数/Parameters:
//   - rawURL: 要读取 Cookie 的 URL。 URL to read cookies for.
func (cm *CookieManager) GetCookies(rawURL string) []*http.Cookie {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	return cm.jar.Cookies(parsedURL)
}

// UpdateCookie 按名称更新配置中的 Cookie 值。
// UpdateCookie updates a named cookie value in the configuration.
// 参数/Parameters:
//   - name: Cookie 名称。 Cookie name.
//   - value: Cookie 新值。 New cookie value.
func (cm *CookieManager) UpdateCookie(name, value string) {
	if cm.config == nil {
		cm.config = &cookieConfig{}
	}

	switch name {
	case "douyin":
		cm.config.Cookie.Douyin = value
	}
}

// SaveConfig 将当前配置保存到文件。
// SaveConfig saves the current configuration to a file.
// 参数/Parameters:
//   - path: YAML 配置文件路径。 YAML config file path.
func (cm *CookieManager) SaveConfig(path string) error {
	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ValidateCookie 简单检查 Cookie 是否包含关键抖音字段。
// ValidateCookie performs a lightweight check for important Douyin cookie fields.
// 参数/Parameters:
//   - cookieStr: 待检查的 Cookie 字符串。 Cookie string to validate.
func (cm *CookieManager) ValidateCookie(cookieStr string) bool {
	if cookieStr == "" {
		return false
	}

	return strings.Contains(cookieStr, "ttwid=") ||
		strings.Contains(cookieStr, "passport_csrf_token=") ||
		strings.Contains(cookieStr, "odin_tt=")
}

// GetCookieNames 返回 Cookie 字符串中包含的所有名称。
// GetCookieNames returns all cookie names contained in a cookie string.
// 参数/Parameters:
//   - cookieStr: Cookie 请求头字符串。 Cookie header string.
func (cm *CookieManager) GetCookieNames(cookieStr string) []string {
	var names []string
	cookies := cm.ParseCookies(cookieStr)
	for _, c := range cookies {
		names = append(names, c.Name)
	}
	return names
}
