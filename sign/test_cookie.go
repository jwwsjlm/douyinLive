package main

import (
	"fmt"
	"net/http"
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
}

// NewCookieManager 创建 Cookie 管理器
func NewCookieManager() *CookieManager {
	return &CookieManager{}
}

// LoadConfig 从 YAML 文件加载配置
func (cm *CookieManager) LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config CookieConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	cm.config = &config
	return nil
}

// GetDouyinCookie 获取抖音 Cookie
func (cm *CookieManager) GetDouyinCookie() string {
	if cm.config != nil {
		return cm.config.Cookie.Douyin
	}
	return ""
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

func main() {
	fmt.Println("🧪 开始测试 Cookie 管理器...")
	fmt.Println()

	// 创建 Cookie 管理器
	cm := NewCookieManager()

	// 测试 1: 从配置文件加载
	fmt.Println("📋 测试 1: 从配置文件加载 Cookie")
	err := cm.LoadConfig("config.yaml")
	if err != nil {
		fmt.Printf("⚠️  配置文件加载失败：%v\n", err)
		fmt.Println("💡 提示：请复制 config.example.yaml 为 config.yaml 并填入 Cookie")
	} else {
		fmt.Println("✅ 配置文件加载成功")
	}
	fmt.Println()

	// 测试 2: 获取抖音 Cookie
	fmt.Println("🍪 测试 2: 获取抖音 Cookie")
	douyinCookie := cm.GetDouyinCookie()
	if douyinCookie == "" {
		fmt.Println("⚠️  Cookie 为空，请使用示例 Cookie 测试")
		// 使用示例 Cookie
		douyinCookie = "ttwid=1%7C2iDIYVmjzMcpZ20fcaFde0VghXAA3NaNXE_SLR68IyE%7C1761045455%7Cab35197d5cfb21df6cbb2fa7ef1c9262206b062c315b9d04da746d0b37dfbc7d; my_rd=1; passport_csrf_token=3ab34460fa656183fccfb904b16ff742; d_ticket=9f562383ac0547d0b561904513229d76c9c21"
		fmt.Println("✅ 使用示例 Cookie")
	} else {
		fmt.Println("✅ 从配置获取 Cookie 成功")
	}
	fmt.Println()

	// 测试 3: 验证 Cookie 有效性
	fmt.Println("✔️  测试 3: 验证 Cookie 有效性")
	isValid := cm.ValidateCookie(douyinCookie)
	if isValid {
		fmt.Println("✅ Cookie 格式有效")
	} else {
		fmt.Println("❌ Cookie 格式无效")
	}
	fmt.Println()

	// 测试 4: 解析 Cookie
	fmt.Println("🔧 测试 4: 解析 Cookie 为 http.Cookie 数组")
	cookies := cm.ParseCookies(douyinCookie)
	fmt.Printf("✅ 解析成功，共 %d 个 Cookie:\n", len(cookies))
	for i, c := range cookies {
		fmt.Printf("   %d. %s = %s\n", i+1, c.Name, c.Value)
	}
	fmt.Println()

	// 测试 5: 获取 Cookie 名称列表
	fmt.Println("📝 测试 5: 获取 Cookie 名称列表")
	names := cm.GetCookieNames(douyinCookie)
	fmt.Printf("✅ Cookie 名称: %v\n", names)
	fmt.Println()

	// 测试 6: 显示完整 Cookie 字符串
	fmt.Println("📄 测试 6: 完整 Cookie 字符串")
	fmt.Printf("长度：%d 字符\n", len(douyinCookie))
	fmt.Printf("内容：%s...\n", douyinCookie[:min(100, len(douyinCookie))])
	fmt.Println()

	fmt.Println("🎉 Cookie 管理器测试完成！")
	fmt.Println()

	// 提示下一步
	fmt.Println("📌 下一步:")
	fmt.Println("1. 复制 config.example.yaml 为 config.yaml")
	fmt.Println("2. 编辑 config.yaml，填入你的真实 Cookie")
	fmt.Println("3. 再次运行测试，验证配置是否正确")
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
