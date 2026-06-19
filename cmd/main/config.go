package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var showVersion bool

var validLogLevels = map[string]struct{}{
	"debug": {},
	"info":  {},
	"warn":  {},
	"error": {},
}

const (
	signProviderLocal  = "local"
	signProviderTikHub = "tikhub"
)

// CookieConfig 存储 Cookie 配置
type CookieConfig struct {
	Douyin string            // 抖音默认 Cookie
	Rooms  map[string]string // 按直播间 ID 配置的 Cookie
}

// MonitorConfig 存储未开播监控相关配置
type MonitorConfig struct {
	PollInterval   time.Duration
	NotifyInterval time.Duration
}

// LogConfig 存储日志配置
type LogConfig struct {
	Level string
}

// SignConfig 存储 WebSocket 签名来源配置
type SignConfig struct {
	Provider string
}

// TikHubConfig 存储 TikHub API 配置
type TikHubConfig struct {
	Key string
}

// Config 存储应用的所有配置
type Config struct {
	Port    string
	Unknown bool
	Cookie  CookieConfig
	Monitor MonitorConfig
	Log     LogConfig
	Sign    SignConfig
	TikHub  TikHubConfig
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizeSignProvider(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = defaultSignProvider
	}

	switch provider {
	case "local", "js", "javascript":
		return signProviderLocal, nil
	case "tikhub", "tik-hub", "tik_hub":
		return signProviderTikHub, nil
	default:
		return "", fmt.Errorf("sign.provider 配置无效: %s，可选值: local, tikhub", provider)
	}
}

// NewConfig 创建并加载应用配置
func NewConfig() (*Config, error) {
	// 绑定命令行参数
	pflag.String("port", "1088", "WebSocket 服务端口")
	pflag.Bool("unknown", false, "是否输出未知源的 pb 消息")
	pflag.String("log-level", "info", "日志级别: debug, info, warn, error")
	pflag.String("sign-provider", defaultSignProvider, "WebSocket 签名来源: local, tikhub")
	pflag.String("tikhub-key", "", "TikHub API Key，用于在线生成 WebSocket xb 签名")
	configFile := pflag.String("config", "", "指定配置文件路径")
	pflag.BoolVar(&showVersion, "version", false, "Print version information")
	pflag.Parse()
	if showVersion {
		fmt.Println(VersionString())
		os.Exit(0)
	}

	// 绑定到 viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	// 配置文件设置
	if *configFile != "" {
		viper.SetConfigFile(*configFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// ✅ 获取 exe 所在目录（解决双击闪退问题）
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			viper.AddConfigPath(exeDir)
		}

		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.app")
		viper.AddConfigPath("/etc/app/")
	}

	// 环境变量支持
	viper.SetEnvPrefix("APP")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// 设置默认值
	viper.SetDefault("port", "1088")
	viper.SetDefault("unknown", false)
	viper.SetDefault("cookie.douyin", "")
	viper.SetDefault("cookie.rooms", map[string]string{})
	viper.SetDefault("monitor.poll_interval", "15s")
	viper.SetDefault("monitor.notify_interval", "30s")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("sign.provider", defaultSignProvider)
	viper.SetDefault("tikhub.key", "")
	// 读取配置
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("读取配置文件失败：%w", err)
		}
		// 配置文件不存在时给出提示
		fmt.Println("⚠️  配置文件未找到，使用默认值或命令行参数")
		fmt.Println("💡 建议在同目录下创建 config.yaml 文件")
	} else {
		fmt.Printf("✅ 使用配置文件：%s\n", viper.ConfigFileUsed())
	}

	pollInterval, err := time.ParseDuration(viper.GetString("monitor.poll_interval"))
	if err != nil {
		return nil, fmt.Errorf("monitor.poll_interval 配置无效：%w", err)
	}
	if pollInterval <= 0 {
		return nil, fmt.Errorf("monitor.poll_interval 必须大于 0")
	}

	notifyInterval, err := time.ParseDuration(viper.GetString("monitor.notify_interval"))
	if err != nil {
		return nil, fmt.Errorf("monitor.notify_interval 配置无效：%w", err)
	}
	if notifyInterval <= 0 {
		return nil, fmt.Errorf("monitor.notify_interval 必须大于 0")
	}

	logLevel := viper.GetString("log.level")
	if flag := pflag.Lookup("log-level"); flag != nil && flag.Changed {
		logLevel = flag.Value.String()
	}
	logLevel = strings.ToLower(firstNonEmpty(logLevel, "info"))
	if _, ok := validLogLevels[logLevel]; !ok {
		return nil, fmt.Errorf("log.level 配置无效: %s", logLevel)
	}

	signProvider := viper.GetString("sign.provider")
	if flag := pflag.Lookup("sign-provider"); flag != nil && flag.Changed {
		signProvider = flag.Value.String()
	}
	signProvider, err = normalizeSignProvider(signProvider)
	if err != nil {
		return nil, err
	}

	tikHubKey := viper.GetString("tikhub.key")
	if flag := pflag.Lookup("tikhub-key"); flag != nil && flag.Changed {
		tikHubKey = flag.Value.String()
	}

	// 填充 Config 结构体
	cfg := &Config{
		Port:    viper.GetString("port"),
		Unknown: viper.GetBool("unknown"),
		Cookie: CookieConfig{
			Douyin: viper.GetString("cookie.douyin"),
			Rooms:  viper.GetStringMapString("cookie.rooms"),
		},
		Monitor: MonitorConfig{
			PollInterval:   pollInterval,
			NotifyInterval: notifyInterval,
		},
		Log: LogConfig{
			Level: logLevel,
		},
		Sign: SignConfig{
			Provider: signProvider,
		},
		TikHub: TikHubConfig{
			Key: strings.TrimSpace(tikHubKey),
		},
	}

	return cfg, nil
}
