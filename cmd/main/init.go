package main

import (
	"errors"
	"github.com/spf13/viper"
	"log"
	"strings"
)

func initConfig() {
	// 设置配置文件名称和路径
	viper.SetConfigName("config")     // 配置文件名称（不带扩展名）
	viper.SetConfigType("yaml")       // 配置文件类型
	viper.AddConfigPath(".")          // 当前目录
	viper.AddConfigPath("$HOME/.app") // 家目录下的.app目录
	viper.AddConfigPath("/etc/app/")  // 系统配置目录

	// 环境变量支持
	viper.SetEnvPrefix("APP")                              // 环境变量前缀
	viper.AutomaticEnv()                                   // 自动绑定环境变量
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // 替换环境变量中的点为下划线

	// 设置默认值
	viper.SetDefault("port", "1088")
	viper.SetDefault("room", "****")
	viper.SetDefault("unknown", false)
	viper.SetDefault("key", "")

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			// 配置文件不存在，使用默认值或命令行参数
			log.Println("配置文件未找到，使用默认值或命令行参数")
		}
	} else {
		log.Printf("使用配置文件: %s", viper.ConfigFileUsed())
	}
}
