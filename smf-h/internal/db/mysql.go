package db

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var Global *gorm.DB

// Config 供简单演示使用，可扩展从环境变量或配置文件加载
type Config struct {
	User     string
	Password string
	Host     string
	Port     int
	Database string
	Charset  string
}

// Init 初始化全局 MySQL 连接
func Init(cfg Config) error {
	if cfg.Charset == "" {
		cfg.Charset = "utf8mb4"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Charset,
	)
	gormCfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)}
	db, err := gorm.Open(mysql.Open(dsn), gormCfg)
	if err != nil {
		return err
	}
	// 设置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(time.Hour)

	fmt.Printf("[DB] connected host=%s port=%d db=%s user=%s charset=%s\n", cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Charset)

	Global = db
	return nil
}
