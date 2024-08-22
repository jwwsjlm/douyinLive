package config

import (
	"log"

	"github.com/spf13/viper"
)

var Conf Config

type Config struct {
	RoomNumber string    `yaml:"roomNumber"`
	DbConf     MySQLConf `yaml:"dbConf"`
}

type MySQLConf struct {
	Username     string
	Password     string
	Host         string
	Database     string
	MaxOpenConns int
	MaxIdleConns int
	MaxLifetime  int
}

func Init() {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")

	err := v.ReadInConfig()
	if err != nil {
		log.Fatalln("Fatal error config file  err: ", err)
	}

	err = v.Unmarshal(&Conf)
	if err != nil {
		log.Fatalln("Fatal error unmarshal config file  err: ", err)
	}
}
