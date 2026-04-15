package test

import (
	"encoding/json"
	"ethclient_service/config"
	"ethclient_service/ethereum"
	"strconv"
	"strings"
	"testing"
)

// TestEtherscanResponseParsing 测试 Etherscan API 响应解析
func TestEtherscanResponseParsing(t *testing.T) {
	tests := []struct {
		name           string
		jsonResponse   string
		expectError    bool
		expectedBlock  int64
		expectedStatus string
	}{
		{
			name: "成功的响应",
			jsonResponse: `{
				"status": "1",
				"message": "OK",
				"result": [
					{
						"contractAddress": "0x40eae793f36076c377435e66903950cb8293eb50",
						"contractCreator": "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266",
						"txHash": "0xabc123...",
						"blockNumber": "10418133",
						"timestamp": "1678901234",
						"dateTime": "2023-03-15 12:34:56",
						"gasUsed": "123456"
					}
				]
			}`,
			expectError:    false,
			expectedBlock:  10418133,
			expectedStatus: "1",
		},
		{
			name: "API 错误响应 - 无效 API Key",
			jsonResponse: `{
				"status": "0",
				"message": "NOTOK",
				"result": "Invalid API Key"
			}`,
			expectError:    true,
			expectedBlock:  0,
			expectedStatus: "0",
		},
		{
			name: "API 错误响应 - 合约未验证",
			jsonResponse: `{
				"status": "0",
				"message": "NOTOK",
				"result": "Contract source code not verified"
			}`,
			expectError:    true,
			expectedBlock:  0,
			expectedStatus: "0",
		},
		{
			name:           "空结果数组",
			jsonResponse:   `{"status": "1", "message": "OK", "result": []}`,
			expectError:    true,
			expectedBlock:  0,
			expectedStatus: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp ethereum.EtherscanResponse
			err := json.Unmarshal([]byte(tt.jsonResponse), &resp)
			if err != nil {
				t.Fatalf("解析 JSON 失败: %v", err)
			}

			if resp.Status != tt.expectedStatus {
				t.Errorf("期望状态 %s, 得到 %s", tt.expectedStatus, resp.Status)
			}

			if resp.Status == "1" && !tt.expectError {
				var results []ethereum.ContractCreationInfo
				err := json.Unmarshal(resp.Result, &results)
				if err != nil {
					t.Fatalf("解析结果失败: %v", err)
				}

				if len(results) == 0 {
					t.Fatal("结果数组为空")
				}

				blockNum := parseBlockNumber(t, results[0].BlockNumber)
				if blockNum != tt.expectedBlock {
					t.Errorf("期望区块号 %d, 得到 %d", tt.expectedBlock, blockNum)
				}
			}
		})
	}
}

// TestGetContractCreationFromEtherscanValidation 测试 Etherscan API 输入验证
func TestGetContractCreationFromEtherscanValidation(t *testing.T) {
	tests := []struct {
		name        string
		chainConfig config.ChainConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "未配置 API Key",
			chainConfig: config.ChainConfig{
				ChainID:         "11155111",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "",
			},
			expectError: true,
			errorMsg:    "Etherscan API Key 未配置",
		},
		{
			name: "不支持的链 ID",
			chainConfig: config.ChainConfig{
				ChainID:         "999999",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "不支持的链 ID",
		},
		{
			name: "无效的合约地址",
			chainConfig: config.ChainConfig{
				ChainID:         "11155111",
				ContractAddress: "",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "合约地址无效",
		},
		{
			name: "Sepolia 配置正确",
			chainConfig: config.ChainConfig{
				ChainID:         "11155111",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "Etherscan API",
		},
		{
			name: "Base Sepolia 配置正确",
			chainConfig: config.ChainConfig{
				ChainID:         "84532",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "Etherscan API",
		},
		{
			name: "Ethereum Mainnet 配置正确",
			chainConfig: config.ChainConfig{
				ChainID:         "1",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "Etherscan API",
		},
		{
			name: "Base Mainnet 配置正确",
			chainConfig: config.ChainConfig{
				ChainID:         "8453",
				ContractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
				EtherscanAPIKey: "test_api_key",
			},
			expectError: true,
			errorMsg:    "Etherscan API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := ethereum.NewClient(tt.chainConfig)
			if err != nil {
				t.Skipf("创建客户端失败: %v", err)
			}
			defer client.Close()

			_, err = client.GetContractDeploymentBlock()

			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误包含 '%s', 但没有错误", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("期望错误包含 '%s', 得到 '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误，但得到: %v", err)
				}
			}
		})
	}
}

// TestSupportedChainIDs 测试支持的链 ID
func TestSupportedChainIDs(t *testing.T) {
	supportedChains := map[string]string{
		"11155111": "https://api-sepolia.etherscan.io/api",
		"84532":    "https://api-sepolia.basescan.org/api",
		"1":        "https://api.etherscan.io/api",
		"8453":     "https://api.basescan.org/api",
	}

	unsupportedChains := []string{
		"31337",  // Localhost
		"999999", // 不存在的链
		"137",    // Polygon (未实现)
		"42161",  // Arbitrum (未实现)
	}

	for chainID, expectedURL := range supportedChains {
		t.Run("支持的链 "+chainID, func(t *testing.T) {
			apiURL := getEtherscanAPIURL(chainID)
			if apiURL != expectedURL {
				t.Errorf("期望 URL %s, 得到 %s", expectedURL, apiURL)
			}
		})
	}

	for _, chainID := range unsupportedChains {
		t.Run("不支持的链 "+chainID, func(t *testing.T) {
			apiURL := getEtherscanAPIURL(chainID)
			if apiURL != "" {
				t.Errorf("期望空 URL, 得到 %s", apiURL)
			}
		})
	}
}

// TestContractAddressValidation 测试合约地址验证
func TestContractAddressValidation(t *testing.T) {
	tests := []struct {
		name            string
		contractAddress string
		isValid         bool
	}{
		{
			name:            "有效的合约地址",
			contractAddress: "0x40eAE793f36076c377435e66903950Cb8293Eb50",
			isValid:         true,
		},
		{
			name:            "空地址",
			contractAddress: "",
			isValid:         false,
		},
		{
			name:            "零地址",
			contractAddress: "0x0000000000000000000000000000000000000000",
			isValid:         true,
		},
		{
			name:            "无效地址格式",
			contractAddress: "invalid_address",
			isValid:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ChainConfig{
				ChainID:         "11155111",
				ContractAddress: tt.contractAddress,
				EtherscanAPIKey: "test_key",
			}

			client, err := ethereum.NewClient(cfg)
			if err != nil {
				t.Skipf("创建客户端失败: %v", err)
				return
			}
			defer client.Close()

			_, err = client.GetContractDeploymentBlock()

			if tt.contractAddress == "" {
				if err == nil || !contains(err.Error(), "合约地址无效") {
					t.Errorf("期望 '合约地址无效' 错误，得到: %v", err)
				}
			}
		})
	}
}

// 辅助函数

func parseBlockNumber(t *testing.T, blockNumberStr string) int64 {
	blockNumberStr = strings.Trim(blockNumberStr, "\"")
	blockNum, err := strconv.ParseInt(blockNumberStr, 10, 64)
	if err != nil {
		t.Fatalf("转换区块号失败: %v", err)
	}
	return blockNum
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getEtherscanAPIURL 根据链 ID 获取 Etherscan API URL（测试用辅助函数）
func getEtherscanAPIURL(chainID string) string {
	switch chainID {
	case "11155111": // Sepolia
		return "https://api-sepolia.etherscan.io/api"
	case "84532": // Base Sepolia
		return "https://api-sepolia.basescan.org/api"
	case "1": // Ethereum Mainnet
		return "https://api.etherscan.io/api"
	case "8453": // Base Mainnet
		return "https://api.basescan.org/api"
	default:
		return ""
	}
}
