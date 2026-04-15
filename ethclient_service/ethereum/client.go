package ethereum

import (
	"context"
	"encoding/json"
	"ethclient_service/config"
	"ethclient_service/logger"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ERC20 ABI (仅包含 Transfer 事件)
const erc20ABI = `[{"anonymous":false,"inputs":[{"indexed":true,"name":"from","type":"address"},{"indexed":true,"name":"to","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]`

// Client 以太坊客户端
type Client struct {
	client          *ethclient.Client
	chainConfig     config.ChainConfig
	contractAddress common.Address
	transferEventID common.Hash
	abi             abi.ABI
}

// TransferEvent Transfer 事件数据结构
type TransferEvent struct {
	From        string
	To          string
	Value       *big.Int
	BlockNumber int64
	BlockHash   string
	TxHash      string
	LogIndex    int
}

// NewClient 创建以太坊客户端
func NewClient(chainConfig config.ChainConfig) (*Client, error) {
	client, err := ethclient.Dial(chainConfig.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("连接以太坊节点失败: %w", err)
	}

	// 解析 ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, fmt.Errorf("解析 ABI 失败: %w", err)
	}

	// 获取 Transfer 事件主题
	transferEvent := parsedABI.Events["Transfer"]

	return &Client{
		client:          client,
		chainConfig:     chainConfig,
		contractAddress: common.HexToAddress(chainConfig.ContractAddress),
		transferEventID: transferEvent.ID,
		abi:             parsedABI,
	}, nil
}

// Close 关闭客户端连接
func (c *Client) Close() {
	c.client.Close()
}

// GetCurrentBlockNumber 获取当前区块号
func (c *Client) GetCurrentBlockNumber() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockNumber, err := c.client.BlockNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取当前区块号失败: %w", err)
	}

	return int64(blockNumber), nil
}

// GetBlockTimestamp 获取区块时间戳
func (c *Client) GetBlockTimestamp(blockNumber int64) (time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	block, err := c.client.BlockByNumber(ctx, big.NewInt(blockNumber))
	if err != nil {
		return time.Time{}, fmt.Errorf("获取区块信息失败: %w", err)
	}

	return time.Unix(int64(block.Time()), 0), nil
}

// GetTransferEvents 获取 Transfer 事件
func (c *Client) GetTransferEvents(fromBlock, toBlock int64) ([]TransferEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 构建查询过滤器
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(toBlock),
		Addresses: []common.Address{c.contractAddress},
		Topics:    [][]common.Hash{{c.transferEventID}},
	}

	// 查询日志
	logs, err := c.client.FilterLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询事件日志失败: %w", err)
	}

	var events []TransferEvent
	for _, vLog := range logs {
		event, err := c.parseTransferLog(vLog)
		if err != nil {
			logger.Log.Warnf("解析事件日志失败: %v", err)
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// parseTransferLog 解析 Transfer 事件日志
func (c *Client) parseTransferLog(vLog types.Log) (TransferEvent, error) {
	event := TransferEvent{
		BlockNumber: int64(vLog.BlockNumber),
		BlockHash:   vLog.BlockHash.Hex(),
		TxHash:      vLog.TxHash.Hex(),
		LogIndex:    int(vLog.Index),
	}

	// 解析事件数据
	if len(vLog.Topics) < 3 {
		return event, fmt.Errorf("事件主题数量不足")
	}

	// from 地址 (indexed)
	from := common.HexToAddress(vLog.Topics[1].Hex())
	event.From = strings.ToLower(from.Hex())

	// to 地址 (indexed)
	to := common.HexToAddress(vLog.Topics[2].Hex())
	event.To = strings.ToLower(to.Hex())

	// value (非 indexed，在 Data 中)
	if len(vLog.Data) >= 32 {
		value := new(big.Int).SetBytes(vLog.Data)
		event.Value = value
	}

	return event, nil
}

// GetContractDeploymentBlock 获取合约部署区块号
// 使用 Etherscan API 获取合约创建信息
func (c *Client) GetContractDeploymentBlock() (int64, error) {
	// 检查合约地址是否有效
	if c.contractAddress == (common.Address{}) {
		return 0, fmt.Errorf("合约地址无效")
	}

	// 从 Etherscan API 获取合约创建区块
	return c.getContractCreationFromEtherscan()
}

// EtherscanResponse Etherscan API 响应结构
type EtherscanResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"` // 使用 RawMessage 处理多种格式
}

// ContractCreationInfo 合约创建信息
type ContractCreationInfo struct {
	ContractAddress string `json:"contractAddress"`
	ContractCreator string `json:"contractCreator"`
	TxHash          string `json:"txHash"`
	BlockNumber     string `json:"blockNumber"`
	Timestamp       string `json:"timestamp"`
	DateTime        string `json:"dateTime"`
	GasUsed         string `json:"gasUsed"`
}

// getContractCreationFromEtherscan 从 Etherscan API V2 获取合约创建信息
func (c *Client) getContractCreationFromEtherscan() (int64, error) {
	apiKey := c.chainConfig.EtherscanAPIKey
	if apiKey == "" {
		return 0, fmt.Errorf("Etherscan API Key 未配置，请配置 ETHERSCAN_API_KEY 或 BASESCAN_API_KEY")
	}

	// V2 API 使用统一的 base URL，通过 chainid 参数指定链
	// 参考: https://docs.etherscan.io/v2-migration
	var chainId string
	switch c.chainConfig.ChainID {
	case "11155111": // Sepolia
		chainId = "11155111"
	case "84532": // Base Sepolia
		chainId = "84532"
	case "1": // Ethereum Mainnet
		chainId = "1"
	case "8453": // Base Mainnet
		chainId = "8453"
	default:
		return 0, fmt.Errorf("不支持的链 ID: %s，无法使用 Etherscan API", c.chainConfig.ChainID)
	}

	// V2 API 统一使用 https://api.etherscan.io/v2/api
	apiURL := "https://api.etherscan.io/v2/api"

	// 构建请求 URL (V2 API 格式) - 注意使用 chainid (小写)
	contractAddr := strings.ToLower(c.contractAddress.Hex())
	url := fmt.Sprintf("%s?chainid=%s&module=contract&action=getcontractcreation&contractaddresses=%s&apikey=%s",
		apiURL, chainId, contractAddr, apiKey)

	// 发送 HTTP 请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("Etherscan API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var etherscanResp EtherscanResponse
	if err := json.NewDecoder(resp.Body).Decode(&etherscanResp); err != nil {
		return 0, fmt.Errorf("解析 Etherscan API 响应失败: %w", err)
	}

	// 检查响应状态
	if etherscanResp.Status != "1" {
		// 尝试解析错误信息（result 可能是字符串）
		var errorMsg string
		if err := json.Unmarshal(etherscanResp.Result, &errorMsg); err == nil {
			return 0, fmt.Errorf("Etherscan API 错误: %s - %s", etherscanResp.Message, errorMsg)
		}
		return 0, fmt.Errorf("Etherscan API 错误: %s", etherscanResp.Message)
	}

	// 解析结果数组
	var results []ContractCreationInfo
	if err := json.Unmarshal(etherscanResp.Result, &results); err != nil {
		return 0, fmt.Errorf("解析合约创建信息失败: %w", err)
	}

	if len(results) == 0 {
		return 0, fmt.Errorf("Etherscan API 未返回合约创建信息")
	}

	// 解析区块号
	blockNum, err := strconv.ParseInt(results[0].BlockNumber, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("解析区块号失败: %w", err)
	}

	logger.Log.Infof("📡 从 Etherscan 获取到合约 %s 的部署区块: %d", contractAddr, blockNum)
	return blockNum, nil
}

// ClientManager 客户端管理器
type ClientManager struct {
	clients map[string]*Client
}

// NewClientManager 创建客户端管理器
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*Client),
	}
}

// AddClient 添加客户端
func (cm *ClientManager) AddClient(name string, chainConfig config.ChainConfig) error {
	client, err := NewClient(chainConfig)
	if err != nil {
		return err
	}
	cm.clients[name] = client
	logger.Log.Infof("链 %s 客户端初始化成功", name)
	return nil
}

// GetClient 获取客户端
func (cm *ClientManager) GetClient(name string) (*Client, bool) {
	client, ok := cm.clients[name]
	return client, ok
}

// CloseAll 关闭所有客户端
func (cm *ClientManager) CloseAll() {
	for name, client := range cm.clients {
		client.Close()
		logger.Log.Infof("链 %s 客户端已关闭", name)
	}
}

// GetAllClients 获取所有客户端
func (cm *ClientManager) GetAllClients() map[string]*Client {
	return cm.clients
}
