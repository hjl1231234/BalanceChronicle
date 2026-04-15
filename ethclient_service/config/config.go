package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// ChainConfig 链配置
type ChainConfig struct {
	ChainID            string `mapstructure:"chain_id"` // 改为string类型
	Name               string `mapstructure:"name"`
	RPCURL             string `mapstructure:"rpc_url"`
	ContractAddress    string `mapstructure:"contract_address"`
	BlockConfirmations int    `mapstructure:"block_confirmations"`
	IsActive           bool   `mapstructure:"is_active"`
	ContractBlock      int64  `mapstructure:"contract_block"`    // 合约部署区块号，0=使用Etherscan API或从创世区块开始
	EtherscanAPIKey    string `mapstructure:"etherscan_api_key"` // Etherscan API Key，用于获取合约创建信息
}

// PointsConfig 积分计算配置
type PointsConfig struct {
	Rate                float64 `mapstructure:"rate"`
	CalculationInterval int     `mapstructure:"calculation_interval_ms"`
}

// EventListenerConfig 事件监听配置
type EventListenerConfig struct {
	PollInterval int `mapstructure:"poll_interval_ms"`
	BatchSize    int `mapstructure:"batch_size"`
}

// Config 应用配置
type Config struct {
	ServerPort  string `mapstructure:"SERVER_PORT"`
	Environment string `mapstructure:"ENVIRONMENT"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`

	// 数据库配置
	DatabaseHost      string `mapstructure:"DATABASE_HOST"`
	DatabasePort      string `mapstructure:"DATABASE_PORT"`
	DatabaseUser      string `mapstructure:"DATABASE_USER"`
	DatabasePassword  string `mapstructure:"DATABASE_PASSWORD"`
	DatabaseName      string `mapstructure:"DATABASE_NAME"`
	DBMaxOpenConns    int    `mapstructure:"DB_MAX_OPEN_CONNS"`
	DBMaxIdleConns    int    `mapstructure:"DB_MAX_IDLE_CONNS"`
	DBConnMaxLifetime int    `mapstructure:"DB_CONN_MAX_LIFETIME_HOURS"`
	DBDropTables      bool   `mapstructure:"DB_DROP_TABLES"` // 启动时删除所有表并重建

	// 多链配置
	Chains map[string]ChainConfig

	// 积分配置
	Points PointsConfig

	// 事件监听配置
	EventListener EventListenerConfig
}

func LoadConfig(path string) (config Config, err error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("路径解析失败: %w", err)
	}

	// 构建完整环境文件路径
	envPath := filepath.Join(absPath, ".env")

	// 设置Viper
	viper.SetConfigFile(envPath)
	viper.AutomaticEnv()

	// 读取配置文件（如果存在）
	if _, err := os.Stat(envPath); err == nil {
		if err = viper.ReadInConfig(); err != nil {
			return config, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, fmt.Errorf("配置解析失败: %w", err)
	}

	// 设置默认值
	if config.Environment == "" {
		config.Environment = "development"
	}
	if config.ServerPort == "" {
		config.ServerPort = "3000"
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}

	// 数据库连接池默认值
	if config.DBMaxOpenConns <= 0 {
		config.DBMaxOpenConns = 50
	}
	if config.DBMaxIdleConns <= 0 {
		config.DBMaxIdleConns = 20
	}
	if config.DBConnMaxLifetime <= 0 {
		config.DBConnMaxLifetime = 1
	}

	// 加载多链配置
	config.Chains = loadChainConfigs()

	// 加载积分配置
	config.Points = loadPointsConfig()

	// 加载事件监听配置
	config.EventListener = loadEventListenerConfig()

	return config, nil
}

// loadChainConfigs 加载多链配置
func loadChainConfigs() map[string]ChainConfig {
	chains := make(map[string]ChainConfig)

	// Sepolia 配置
	if viper.GetBool("SEPOLIA_ENABLED") {
		contractBlock := viper.GetInt64("SEPOLIA_CONTRACT_BLOCK")
		chains["sepolia"] = ChainConfig{
			ChainID:            "11155111",
			Name:               "Sepolia",
			RPCURL:             viper.GetString("SEPOLIA_RPC_URL"),
			ContractAddress:    strings.ToLower(viper.GetString("SEPOLIA_CONTRACT_ADDRESS")),
			BlockConfirmations: 6,
			IsActive:           true,
			ContractBlock:      contractBlock,
			EtherscanAPIKey:    viper.GetString("ETHERSCAN_API_KEY"),
		}
	}

	// Base Sepolia 配置
	if viper.GetBool("BASE_SEPOLIA_ENABLED") {
		contractBlock := viper.GetInt64("BASE_SEPOLIA_CONTRACT_BLOCK")
		chains["base_sepolia"] = ChainConfig{
			ChainID:            "84532",
			Name:               "Base Sepolia",
			RPCURL:             viper.GetString("BASE_SEPOLIA_RPC_URL"),
			ContractAddress:    strings.ToLower(viper.GetString("BASE_SEPOLIA_CONTRACT_ADDRESS")),
			BlockConfirmations: 6,
			IsActive:           true,
			ContractBlock:      contractBlock,
			EtherscanAPIKey:    viper.GetString("BASESCAN_API_KEY"),
		}
	}

	// Localhost 配置
	if viper.GetBool("LOCALHOST_ENABLED") {
		confirmations := viper.GetInt("LOCALHOST_BLOCK_CONFIRMATIONS")
		if confirmations <= 0 {
			confirmations = 1
		}
		contractBlock := viper.GetInt64("LOCALHOST_CONTRACT_BLOCK")
		chains["localhost"] = ChainConfig{
			ChainID:            "31337",
			Name:               "Localhost",
			RPCURL:             viper.GetString("LOCALHOST_RPC_URL"),
			ContractAddress:    strings.ToLower(viper.GetString("LOCALHOST_CONTRACT_ADDRESS")),
			BlockConfirmations: confirmations,
			IsActive:           true,
			ContractBlock:      contractBlock,
			EtherscanAPIKey:    "", // 本地网络不使用 Etherscan
		}
	}

	return chains
}

// loadPointsConfig 加载积分配置
func loadPointsConfig() PointsConfig {
	rate, _ := strconv.ParseFloat(viper.GetString("POINTS_RATE"), 64)
	if rate <= 0 {
		rate = 0.05
	}

	interval := viper.GetInt("POINTS_CALCULATION_INTERVAL")
	if interval <= 0 {
		interval = 3600000 // 默认1小时
	}

	return PointsConfig{
		Rate:                rate,
		CalculationInterval: interval,
	}
}

// loadEventListenerConfig 加载事件监听配置
func loadEventListenerConfig() EventListenerConfig {
	pollInterval := viper.GetInt("EVENT_POLL_INTERVAL")
	if pollInterval <= 0 {
		pollInterval = 5000 // 默认5秒
	}

	batchSize := viper.GetInt("EVENT_BATCH_SIZE")
	if batchSize <= 0 {
		batchSize = 100
	}

	return EventListenerConfig{
		PollInterval: pollInterval,
		BatchSize:    batchSize,
	}
}

// GetActiveChains 获取激活的链配置
func (c *Config) GetActiveChains() map[string]ChainConfig {
	active := make(map[string]ChainConfig)
	for name, chain := range c.Chains {
		if chain.IsActive && chain.ContractAddress != "" {
			active[name] = chain
		}
	}
	return active
}
