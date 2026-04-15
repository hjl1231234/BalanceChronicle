package test

import (
	"ethclient_service/config"
	"ethclient_service/ethereum"
	"ethclient_service/logger"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func init() {
	// 初始化 logger，避免 nil pointer
	logger.Log = logrus.New()
	logger.Log.SetLevel(logrus.InfoLevel)
}

// getProjectRoot 获取项目根目录
func getProjectRoot() string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(b), "..")
}

// loadEnv 加载环境变量配置文件
func loadEnv(t *testing.T) {
	projectRoot := getProjectRoot()
	envPath := filepath.Join(projectRoot, ".env")

	viper.SetConfigFile(envPath)
	viper.AutomaticEnv()

	if _, err := os.Stat(envPath); err == nil {
		if err := viper.ReadInConfig(); err != nil {
			t.Logf("警告: 读取 .env 文件失败: %v", err)
		}
	}
}

// TestRealEtherscanAPI_Sepolia 使用真实的 ETHERSCAN_API_KEY 测试 Sepolia 合约
func TestRealEtherscanAPI_Sepolia(t *testing.T) {
	loadEnv(t)

	apiKey := viper.GetString("ETHERSCAN_API_KEY")
	contractAddress := viper.GetString("SEPOLIA_CONTRACT_ADDRESS")
	rpcURL := viper.GetString("SEPOLIA_RPC_URL")

	if apiKey == "" {
		t.Skip("跳过测试: ETHERSCAN_API_KEY 未设置")
	}
	if contractAddress == "" {
		t.Skip("跳过测试: SEPOLIA_CONTRACT_ADDRESS 未设置")
	}
	if rpcURL == "" {
		t.Skip("跳过测试: SEPOLIA_RPC_URL 未设置")
	}

	t.Logf("使用合约地址: %s", contractAddress)
	t.Logf("RPC URL: %s", rpcURL)
	t.Logf("API Key 前10位: %s...", apiKey[:min(10, len(apiKey))])

	chainConfig := config.ChainConfig{
		ChainID:         "11155111",
		Name:            "Sepolia",
		RPCURL:          rpcURL,
		ContractAddress: contractAddress,
		EtherscanAPIKey: apiKey,
	}

	client, err := ethereum.NewClient(chainConfig)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	blockNumber, err := client.GetContractDeploymentBlock()
	if err != nil {
		t.Fatalf("获取合约部署区块失败: %v", err)
	}

	if blockNumber <= 0 {
		t.Errorf("获取的区块号无效: %d", blockNumber)
	}

	t.Logf("✅ Sepolia 合约 %s 的部署区块号: %d", contractAddress, blockNumber)
}

// TestRealEtherscanAPI_BaseSepolia 使用真实的 ETHERSCAN_API_KEY 测试 Base Sepolia 合约
func TestRealEtherscanAPI_BaseSepolia(t *testing.T) {
	loadEnv(t)

	// Base Sepolia 使用 ETHERSCAN_API_KEY 或 BASESCAN_API_KEY
	apiKey := viper.GetString("BASESCAN_API_KEY")
	if apiKey == "" {
		apiKey = viper.GetString("ETHERSCAN_API_KEY")
	}
	contractAddress := viper.GetString("BASE_SEPOLIA_CONTRACT_ADDRESS")
	rpcURL := viper.GetString("BASE_SEPOLIA_RPC_URL")

	if apiKey == "" {
		t.Skip("跳过测试: ETHERSCAN_API_KEY 或 BASESCAN_API_KEY 未设置")
	}
	if contractAddress == "" {
		t.Skip("跳过测试: BASE_SEPOLIA_CONTRACT_ADDRESS 未设置")
	}
	if rpcURL == "" {
		t.Skip("跳过测试: BASE_SEPOLIA_RPC_URL 未设置")
	}

	t.Logf("使用合约地址: %s", contractAddress)
	t.Logf("RPC URL: %s", rpcURL)
	t.Logf("API Key 前10位: %s...", apiKey[:min(10, len(apiKey))])

	chainConfig := config.ChainConfig{
		ChainID:         "84532",
		Name:            "Base Sepolia",
		RPCURL:          rpcURL,
		ContractAddress: contractAddress,
		EtherscanAPIKey: apiKey,
	}

	client, err := ethereum.NewClient(chainConfig)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	blockNumber, err := client.GetContractDeploymentBlock()
	if err != nil {
		t.Fatalf("获取合约部署区块失败: %v", err)
	}

	if blockNumber <= 0 {
		t.Errorf("获取的区块号无效: %d", blockNumber)
	}

	t.Logf("✅ Base Sepolia 合约 %s 的部署区块号: %d", contractAddress, blockNumber)
}

// TestRealEtherscanAPI_BothContracts 同时测试两个合约
func TestRealEtherscanAPI_BothContracts(t *testing.T) {
	loadEnv(t)

	etherscanAPIKey := viper.GetString("ETHERSCAN_API_KEY")
	basescanAPIKey := viper.GetString("BASESCAN_API_KEY")
	if basescanAPIKey == "" {
		basescanAPIKey = etherscanAPIKey
	}

	sepoliaAddress := viper.GetString("SEPOLIA_CONTRACT_ADDRESS")
	baseSepoliaAddress := viper.GetString("BASE_SEPOLIA_CONTRACT_ADDRESS")
	sepoliaRPC := viper.GetString("SEPOLIA_RPC_URL")
	baseSepoliaRPC := viper.GetString("BASE_SEPOLIA_RPC_URL")

	tests := []struct {
		name            string
		chainID         string
		contractAddress string
		rpcURL          string
		apiKey          string
		skipReason      string
	}{
		{
			name:            "Sepolia",
			chainID:         "11155111",
			contractAddress: sepoliaAddress,
			rpcURL:          sepoliaRPC,
			apiKey:          etherscanAPIKey,
			skipReason:      "ETHERSCAN_API_KEY、SEPOLIA_CONTRACT_ADDRESS 或 SEPOLIA_RPC_URL 未设置",
		},
		{
			name:            "Base Sepolia",
			chainID:         "84532",
			contractAddress: baseSepoliaAddress,
			rpcURL:          baseSepoliaRPC,
			apiKey:          basescanAPIKey,
			skipReason:      "BASESCAN_API_KEY/ETHERSCAN_API_KEY、BASE_SEPOLIA_CONTRACT_ADDRESS 或 BASE_SEPOLIA_RPC_URL 未设置",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.apiKey == "" || tt.contractAddress == "" || tt.rpcURL == "" {
				t.Skipf("跳过测试: %s", tt.skipReason)
			}

			t.Logf("测试 %s 链", tt.name)
			t.Logf("合约地址: %s", tt.contractAddress)
			t.Logf("RPC URL: %s", tt.rpcURL)

			chainConfig := config.ChainConfig{
				ChainID:         tt.chainID,
				Name:            tt.name,
				RPCURL:          tt.rpcURL,
				ContractAddress: tt.contractAddress,
				EtherscanAPIKey: tt.apiKey,
			}

			client, err := ethereum.NewClient(chainConfig)
			if err != nil {
				t.Fatalf("创建客户端失败: %v", err)
			}
			defer client.Close()

			blockNumber, err := client.GetContractDeploymentBlock()
			if err != nil {
				t.Fatalf("获取合约部署区块失败: %v", err)
			}

			if blockNumber <= 0 {
				t.Errorf("获取的区块号无效: %d", blockNumber)
			}

			t.Logf("✅ %s 合约 %s 的部署区块号: %d", tt.name, tt.contractAddress, blockNumber)
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
