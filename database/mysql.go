package database

import (
	"DouyinLive/config"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var dbMap map[string]*gorm.DB
var DB *gorm.DB

func InitRMSDB(conf config.MySQLConf) {
	DB = initDB(conf)
}

func initDB(conf config.MySQLConf) *gorm.DB {
	if conf == (config.MySQLConf{}) {
		panic("init gorm failed, please confirm the MySQL config is set correctly")
	}

	//@parseTime MySQL中的DATE、DATETIME、TIMESTAMP等时间类型字段将自动转换为golang中的time.Time类型。
	//@loc 设置转换为 time.Time 类型时,使用的时区信息
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8&parseTime=True&loc=UTC",
		conf.Username, conf.Password, conf.Host, conf.Database)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Info),
		PrepareStmt: true,
		QueryFields: true,
	})
	if err != nil {
		panic(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}

	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	sqlDB.SetMaxIdleConns(conf.MaxIdleConns)

	// SetMaxOpenConns 设置打开数据库连接的最大数量。
	sqlDB.SetMaxOpenConns(conf.MaxOpenConns)

	// SetConnMaxLifetime 设置了连接可复用的最大时间。
	sqlDB.SetConnMaxLifetime(time.Duration(conf.MaxLifetime) * time.Second)

	addDBMap(conf.Database, db)
	return db
}

func addDBMap(database string, db *gorm.DB) {
	if dbMap == nil {
		dbMap = make(map[string]*gorm.DB)
	}
	dbMap[database] = db
}
