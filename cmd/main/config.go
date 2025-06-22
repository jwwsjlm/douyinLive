package main

import (
	"errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"strings"
)

// Config 存储应用的所有配置
type Config struct {
	Port    string
	Unknown bool
	Key     string
}

// NewConfig 创建并加载应用配置
func NewConfig() (*Config, error) {
	// 绑定命令行参数
	pflag.String("port", "1088", "WebSocket 服务端口")
	pflag.Bool("unknown", false, "是否输出未知源的pb消息")
	pflag.String("key", "", "tikhub key")
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
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.app")
		viper.AddConfigPath("/etc/app/")
	}

	// 环境变量支持
	viper.SetEnvPrefix("APP")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取配置
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			// 仅当错误不是“文件未找到”时返回错误
			return nil, err
		}
	}

	// 填充 Config 结构体
	cfg := &Config{
		Port:    viper.GetString("port"),
		Unknown: viper.GetBool("unknown"),
		Key:     viper.GetString("key"),
	}

	return cfg, nil
}
