package sign

import (
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CookieConfig Cookie 配置结构
type CookieConfig struct {
	Cookie struct {
		Douyin   string `yaml:"douyin"`
		Tiktok   string `yaml:"tiktok"`
		Kuaishou string `yaml:"kuaishou"`
		Huya     string `yaml:"huya"`
		Douyu    string `yaml:"douyu"`
		Bilibili string `yaml:"bilibili"`
	} `yaml:"cookie"`
}

// CookieManager Cookie 管理器
type CookieManager struct {
	config *CookieConfig
	jar    *cookiejar.Jar
}

// NewCookieManager 创建 Cookie 管理器
func NewCookieManager() *CookieManager {
	jar, _ := cookiejar.New(nil)
	return &CookieManager{
		jar: jar,
	}
}

// LoadConfig 从 YAML 文件加载配置
func (cm *CookieManager) LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	log.Println("加载配置文件成功", path)
	var config CookieConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	cm.config = &config
	return nil
}

// LoadFromEnv 从环境变量加载 Cookie
func (cm *CookieManager) LoadFromEnv() {
	if cm.config == nil {
		cm.config = &CookieConfig{}
	}

	cm.config.Cookie.Douyin = os.Getenv("DOUYIN_COOKIE")
	cm.config.Cookie.Tiktok = os.Getenv("TIKTOK_COOKIE")
	cm.config.Cookie.Kuaishou = os.Getenv("KUAISHOU_COOKIE")
}

// GetDouyinCookie 获取抖音 Cookie
func (cm *CookieManager) GetDouyinCookie() string {
	if cm.config != nil {
		return cm.config.Cookie.Douyin
	}
	return ""
}

// SetDouyinCookie 手动设置抖音 Cookie
func (cm *CookieManager) SetDouyinCookie(cookie string) {
	if cm.config == nil {
		cm.config = &CookieConfig{}
	}
	cm.config.Cookie.Douyin = cookie
}

// ParseCookies 解析 cookie 字符串为 []*http.Cookie
func (cm *CookieManager) ParseCookies(cookieStr string) []*http.Cookie {
	var cookies []*http.Cookie
	if cookieStr == "" {
		return cookies
	}

	pairs := strings.Split(cookieStr, "; ")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  strings.TrimSpace(parts[0]),
				Value: strings.TrimSpace(parts[1]),
			})
		}
	}

	return cookies
}

// SetCookies 设置 Cookie 到 jar
func (cm *CookieManager) SetCookies(rawURL string, cookieStr string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	cookies := cm.ParseCookies(cookieStr)
	cm.jar.SetCookies(parsedURL, cookies)
	return nil
}

// GetCookies 从 jar 获取 Cookie
func (cm *CookieManager) GetCookies(rawURL string) []*http.Cookie {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	return cm.jar.Cookies(parsedURL)
}

// UpdateCookie 更新指定名称的 Cookie 值
func (cm *CookieManager) UpdateCookie(name, value string) {
	if cm.config != nil {
		switch name {
		case "douyin":
			cm.config.Cookie.Douyin = value
		case "tiktok":
			cm.config.Cookie.Tiktok = value
		case "kuaishou":
			cm.config.Cookie.Kuaishou = value
		}
	}
}

// SaveConfig 保存配置到文件
func (cm *CookieManager) SaveConfig(path string) error {
	data, err := yaml.Marshal(cm.config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// ValidateCookie 验证 Cookie 是否有效（简单检查）
func (cm *CookieManager) ValidateCookie(cookieStr string) bool {
	if cookieStr == "" {
		return false
	}

	// 检查是否包含关键 cookie
	return strings.Contains(cookieStr, "ttwid=") ||
		strings.Contains(cookieStr, "passport_csrf_token=") ||
		strings.Contains(cookieStr, "odin_tt=")
}

// GetCookieNames 获取 Cookie 中包含的所有名称
func (cm *CookieManager) GetCookieNames(cookieStr string) []string {
	var names []string
	cookies := cm.ParseCookies(cookieStr)
	for _, c := range cookies {
		names = append(names, c.Name)
	}
	return names
}
