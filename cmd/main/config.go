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

// CookieConfig 存储 Cookie 配置
type CookieConfig struct {
	Douyin string // 抖音 Cookie
}

// MonitorConfig 存储未开播监控相关配置
type MonitorConfig struct {
	PollInterval   time.Duration
	NotifyInterval time.Duration
}

// PprofConfig 存储 pprof 调试配置
type PprofConfig struct {
	Enabled bool
	Port    string
}

// Config 存储应用的所有配置
type Config struct {
	Port    string
	Unknown bool
	Cookie  CookieConfig
	Monitor MonitorConfig
	Pprof   PprofConfig
}

// NewConfig 创建并加载应用配置
func NewConfig() (*Config, error) {
	// 绑定命令行参数
	pflag.String("port", "1088", "WebSocket 服务端口")
	pflag.Bool("unknown", false, "是否输出未知源的 pb 消息")
	pflag.Bool("pprof", false, "是否启用 pprof 调试服务")
	pflag.String("pprof-port", "6060", "pprof 调试服务端口")
	configFile := pflag.String("config", "", "指定配置文件路径")
	pflag.Parse()

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
	viper.SetDefault("monitor.poll_interval", "15s")
	viper.SetDefault("monitor.notify_interval", "30s")
	viper.SetDefault("pprof.enabled", false)
	viper.SetDefault("pprof.port", "6060")

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

	// 填充 Config 结构体
	cfg := &Config{
		Port:    viper.GetString("port"),
		Unknown: viper.GetBool("unknown"),
		Cookie: CookieConfig{
			Douyin: viper.GetString("cookie.douyin"),
		},
		Monitor: MonitorConfig{
			PollInterval:   pollInterval,
			NotifyInterval: notifyInterval,
		},
		Pprof: PprofConfig{
			Enabled: viper.GetBool("pprof.enabled") || viper.GetBool("pprof"),
			Port:    viper.GetString("pprof.port"),
		},
	}

	return cfg, nil
}
