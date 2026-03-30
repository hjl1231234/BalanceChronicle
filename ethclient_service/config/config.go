package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	ServerPort string `mapstructure:"SERVER_PORT"`

	DatabaseHost     string `mapstructure:"DATABASE_HOST"`
	DatabasePort     string `mapstructure:"DATABASE_PORT"`
	DatabaseUser     string `mapstructure:"DATABASE_USER"`
	DatabasePassword string `mapstructure:"DATABASE_PASSWORD"`
	DatabaseName     string `mapstructure:"DATABASE_NAME"`

	// 数据库连接池配置
	DBMaxOpenConns    int `mapstructure:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns    int `mapstructure:"DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime int `mapstructure:"DB_CONN_MAX_LIFETIME_HOURS"` // 单位: 小时

	LogLevel string `mapstructure:"LOG_LEVEL"`
	// 添加环境标识字段
	Environment string `mapstructure:"ENVIRONMENT"`

	// Ethereum配置
	EthNodeURL           string `mapstructure:"ETH_NODE_URL"`
	TokenContractAddress string `mapstructure:"TOKEN_CONTRACT_ADDRESS"`
	StartBlockNumber     int64  `mapstructure:"START_BLOCK_NUMBER"`
	PollingInterval      int    `mapstructure:"POLLING_INTERVAL_SECONDS"`
	BlockBatchSize       int    `mapstructure:"BLOCK_BATCH_SIZE"`
}

func LoadConfig(path string) (config Config, err error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("路径解析失败: %w", err)
	}

	// 构建完整环境文件路径
	envPath := filepath.Join(absPath, ".env")

	// 检查文件是否存在
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return Config{}, fmt.Errorf("配置文件不存在: %s", envPath)
	}

	// 设置Viper
	viper.SetConfigFile(envPath)

	viper.AutomaticEnv()
	// 读取配置文件

	if err = viper.ReadInConfig(); err != nil {
		return config, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, fmt.Errorf("配置解析失败: %w", err)
	}

	// 如果未设置环境，默认为开发环境
	if config.Environment == "" {
		config.Environment = "development"
	}

	// 设置数据库连接池默认值
	if config.DBMaxOpenConns <= 0 {
		config.DBMaxOpenConns = 50
	}
	if config.DBMaxIdleConns <= 0 {
		config.DBMaxIdleConns = 20
	}
	if config.DBConnMaxLifetime <= 0 {
		config.DBConnMaxLifetime = 1
	}

	// 设置Ethereum配置默认值
	if config.PollingInterval <= 0 {
		config.PollingInterval = 60 // 默认60秒轮询一次
	}
	if config.BlockBatchSize <= 0 {
		config.BlockBatchSize = 1000 // 默认每次处理1000个区块
	}
	return config, nil
}
