package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"strings"
)

// Config 存储应用的所有配置
type Config struct {
	Port    string
	Unknown bool
	Room    string
}

// NewConfig 创建并加载应用配置
func NewConfig() (*Config, error) {
	// 绑定命令行参数
	pflag.String("port", "1088", "WebSocket 服务端口")
	pflag.Bool("unknown", false, "是否输出未知源的 pb 消息")
	pflag.String("room", "", "抖音直播间号")
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
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 设置默认值
	viper.SetDefault("port", "1088")
	viper.SetDefault("unknown", false)

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

	// 填充 Config 结构体
	cfg := &Config{
		Port:    viper.GetString("port"),
		Unknown: viper.GetBool("unknown"),
		Room:    viper.GetString("room"),
	}

	// ✅ 验证必要配置
	if cfg.Room == "" {
		return nil, errors.New("直播间号不能为空，请在 config.yaml 中配置或通过 --room 参数指定")
	}

	return cfg, nil
}
