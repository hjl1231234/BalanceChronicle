package database

import (
	"ethclient_service/config"
	"ethclient_service/logger"
	"ethclient_service/models"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB(cfg *config.Config) error {
	var err error

	// 根据配置选择数据库驱动
	if cfg.DatabaseHost != "" {
		// PostgreSQL
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.DatabaseHost,
			cfg.DatabasePort,
			cfg.DatabaseUser,
			cfg.DatabasePassword,
			cfg.DatabaseName,
		)
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
	} else {
		// SQLite (默认)
		DB, err = gorm.Open(sqlite.Open("ethclient.db"), &gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: false,
			},
		})
	}

	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}

	// 配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库实例失败: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.DBConnMaxLifetime) * time.Hour)

	logger.Log.Info("数据库连接成功")

	// 如果配置了删除表，则先删除所有表
	if cfg.DBDropTables {
		logger.Log.Warn("⚠️  配置要求删除所有表，正在删除...")
		if err := models.DropAllTables(DB); err != nil {
			return fmt.Errorf("删除表失败: %w", err)
		}
		logger.Log.Info("✅ 所有表已删除")
	}

	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
