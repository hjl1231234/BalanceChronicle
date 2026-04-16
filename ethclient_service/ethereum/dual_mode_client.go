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
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ConnectionMode 连接模式
type ConnectionMode int

const (
	ModeHTTP ConnectionMode = iota // HTTP 轮询模式
	ModeWS                         // WebSocket 订阅模式
)

// DualModeClient 双模式以太坊客户端 (WebSocket + HTTP)
type DualModeClient struct {
	cfg             config.ChainConfig
	pollInterval    int // 轮询间隔 (毫秒)
	contractAddress common.Address
	transferEventID common.Hash
	abi             abi.ABI

	// HTTP 客户端 (备用)
	httpClient     *ethclient.Client
	httpClientLock sync.RWMutex

	// WebSocket 客户端 (主用)
	wsClient     *ethclient.Client
	wsSub        ethereum.Subscription
	wsSubActive  bool
	wsClientLock sync.RWMutex

	// 当前模式
	currentMode ConnectionMode
	modeLock    sync.RWMutex

	// 重连退避
	reconnectBackoff time.Duration
	maxBackoff       time.Duration
	backoffLock      sync.Mutex

	// 事件通道
	eventChan chan TransferEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// 状态
	isRunning bool
}

// NewDualModeClient 创建双模式客户端
func NewDualModeClient(cfg config.ChainConfig, pollInterval int) (*DualModeClient, error) {
	// 解析 ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, fmt.Errorf("解析 ABI 失败: %w", err)
	}

	// 获取 Transfer 事件主题
	transferEvent := parsedABI.Events["Transfer"]

	client := &DualModeClient{
		cfg:              cfg,
		pollInterval:     pollInterval,
		contractAddress:  common.HexToAddress(cfg.ContractAddress),
		transferEventID:  transferEvent.ID,
		abi:              parsedABI,
		reconnectBackoff: 1 * time.Second,
		maxBackoff:       60 * time.Second,
		eventChan:        make(chan TransferEvent, 100),
		stopChan:         make(chan struct{}),
		currentMode:      ModeHTTP, // 默认 HTTP 模式
	}

	// 初始化 HTTP 客户端 (必须)
	if err := client.initHTTPClient(); err != nil {
		return nil, err
	}

	return client, nil
}

// initHTTPClient 初始化 HTTP 客户端
func (c *DualModeClient) initHTTPClient() error {
	c.httpClientLock.Lock()
	defer c.httpClientLock.Unlock()

	if c.httpClient != nil {
		c.httpClient.Close()
	}

	client, err := ethclient.Dial(c.cfg.RPCURL)
	if err != nil {
		return fmt.Errorf("连接 HTTP 节点失败: %w", err)
	}

	c.httpClient = client
	logger.Log.Infof("✅ HTTP 客户端初始化成功: %s", c.cfg.Name)
	return nil
}

// initWSClient 初始化 WebSocket 客户端
func (c *DualModeClient) initWSClient() error {
	c.wsClientLock.Lock()
	defer c.wsClientLock.Unlock()

	if c.wsClient != nil {
		c.wsClient.Close()
	}

	if c.cfg.WSRPCURL == "" {
		return fmt.Errorf("WebSocket URL 未配置")
	}

	client, err := ethclient.Dial(c.cfg.WSRPCURL)
	if err != nil {
		return fmt.Errorf("连接 WebSocket 节点失败: %w", err)
	}

	c.wsClient = client
	logger.Log.Infof("✅ WebSocket 客户端初始化成功: %s", c.cfg.Name)
	return nil
}

// Start 启动监听
func (c *DualModeClient) Start() error {
	if c.isRunning {
		return nil
	}

	c.isRunning = true

	// 优先尝试 WebSocket 模式
	if c.cfg.UseWebSocket && c.cfg.WSRPCURL != "" {
		if err := c.startWebSocketListener(); err != nil {
			logger.Log.Warnf("⚠️ %s WebSocket 启动失败，降级为 HTTP 轮询: %v", c.cfg.Name, err)
			c.startHTTPPoller()
		}
	} else {
		logger.Log.Infof("📡 %s 使用 HTTP 轮询模式", c.cfg.Name)
		c.startHTTPPoller()
	}

	return nil
}

// Stop 停止监听
func (c *DualModeClient) Stop() {
	if !c.isRunning {
		return
	}

	close(c.stopChan)
	c.wg.Wait()

	c.httpClientLock.Lock()
	if c.httpClient != nil {
		c.httpClient.Close()
	}
	c.httpClientLock.Unlock()

	c.wsClientLock.Lock()
	if c.wsSub != nil {
		c.wsSub.Unsubscribe()
	}
	if c.wsClient != nil {
		c.wsClient.Close()
	}
	c.wsClientLock.Unlock()

	c.isRunning = false
	logger.Log.Infof("🛑 %s 双模式客户端已停止", c.cfg.Name)
}

// startWebSocketListener 启动 WebSocket 订阅监听
func (c *DualModeClient) startWebSocketListener() error {
	if err := c.initWSClient(); err != nil {
		return err
	}

	c.wsClientLock.RLock()
	wsClient := c.wsClient
	c.wsClientLock.RUnlock()

	// 构建订阅查询
	query := ethereum.FilterQuery{
		Addresses: []common.Address{c.contractAddress},
		Topics:    [][]common.Hash{{c.transferEventID}},
	}

	// 创建订阅
	ctx := context.Background()
	logs := make(chan types.Log)
	sub, err := wsClient.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return fmt.Errorf("订阅事件失败: %w", err)
	}

	c.wsClientLock.Lock()
	c.wsSub = sub
	c.wsSubActive = true
	c.wsClientLock.Unlock()

	c.setMode(ModeWS)
	logger.Log.Infof("🎧 %s WebSocket 事件订阅已启动", c.cfg.Name)

	// 启动处理 goroutine
	c.wg.Add(1)
	go c.handleWebSocketLogs(logs, sub)

	return nil
}

// handleWebSocketLogs 处理 WebSocket 日志
func (c *DualModeClient) handleWebSocketLogs(logs chan types.Log, sub ethereum.Subscription) {
	defer c.wg.Done()

	for {
		select {
		case err := <-sub.Err():
			logger.Log.Errorf("❌ %s WebSocket 订阅错误: %v", c.cfg.Name, err)
			c.wsClientLock.Lock()
			c.wsSubActive = false
			c.wsClientLock.Unlock()

			// 尝试重连
			c.attemptWSReconnect()
			return

		case vLog := <-logs:
			event, err := c.parseTransferLog(vLog)
			if err != nil {
				logger.Log.Warnf("解析事件日志失败: %v", err)
				continue
			}
			c.eventChan <- event

		case <-c.stopChan:
			return
		}
	}
}

// attemptWSReconnect 尝试 WebSocket 重连
func (c *DualModeClient) attemptWSReconnect() {
	c.backoffLock.Lock()
	backoff := c.reconnectBackoff
	c.backoffLock.Unlock()

	logger.Log.Infof("🔄 %s WebSocket 将在 %v 后尝试重连...", c.cfg.Name, backoff)

	time.AfterFunc(backoff, func() {
		if !c.isRunning {
			return
		}

		// 尝试重新启动 WebSocket
		if err := c.startWebSocketListener(); err != nil {
			logger.Log.Errorf("❌ %s WebSocket 重连失败: %v", c.cfg.Name, err)

			// 增加退避时间
			c.backoffLock.Lock()
			c.reconnectBackoff *= 2
			if c.reconnectBackoff > c.maxBackoff {
				c.reconnectBackoff = c.maxBackoff
			}
			c.backoffLock.Unlock()

			// 切换到 HTTP 模式
			c.setMode(ModeHTTP)
			c.startHTTPPoller()
		} else {
			// 重连成功，重置退避
			c.backoffLock.Lock()
			c.reconnectBackoff = 1 * time.Second
			c.backoffLock.Unlock()
		}
	})
}

// startHTTPPoller 启动 HTTP 轮询
func (c *DualModeClient) startHTTPPoller() {
	c.wg.Add(1)
	go c.httpPollingLoop()
}

// httpPollingLoop HTTP 轮询循环
func (c *DualModeClient) httpPollingLoop() {
	defer c.wg.Done()

	c.setMode(ModeHTTP)
	logger.Log.Infof("📡 %s HTTP 轮询已启动", c.cfg.Name)

	ticker := time.NewTicker(time.Duration(c.pollInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			logger.Log.Infof("🛑 %s HTTP 轮询已停止", c.cfg.Name)
			return
		case <-ticker.C:
			// 如果 WebSocket 已激活，跳过 HTTP 轮询
			c.wsClientLock.RLock()
			wsActive := c.wsSubActive
			c.wsClientLock.RUnlock()

			if wsActive {
				continue
			}

			if err := c.pollEvents(); err != nil {
				logger.Log.Errorf("%s HTTP 轮询出错: %v", c.cfg.Name, err)
			}
		}
	}
}

// pollEvents 轮询事件
func (c *DualModeClient) pollEvents() error {
	c.httpClientLock.RLock()
	httpClient := c.httpClient
	c.httpClientLock.RUnlock()

	if httpClient == nil {
		return fmt.Errorf("HTTP 客户端未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 获取当前区块号
	currentBlock, err := httpClient.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("获取当前区块号失败: %w", err)
	}

	safeBlock := int64(currentBlock) - int64(c.cfg.BlockConfirmations)
	if safeBlock < 0 {
		safeBlock = 0
	}

	// 这里需要维护一个 lastSyncedBlock 状态
	// 简化处理：查询最近一批区块
	fromBlock := safeBlock - 10
	if fromBlock < 0 {
		fromBlock = 0
	}

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(safeBlock),
		Addresses: []common.Address{c.contractAddress},
		Topics:    [][]common.Hash{{c.transferEventID}},
	}

	logs, err := httpClient.FilterLogs(ctx, query)
	if err != nil {
		return fmt.Errorf("查询事件日志失败: %w", err)
	}

	for _, vLog := range logs {
		event, err := c.parseTransferLog(vLog)
		if err != nil {
			logger.Log.Warnf("解析事件日志失败: %v", err)
			continue
		}
		select {
		case c.eventChan <- event:
		case <-c.stopChan:
			return nil
		}
	}

	return nil
}

// parseTransferLog 解析 Transfer 事件日志
func (c *DualModeClient) parseTransferLog(vLog types.Log) (TransferEvent, error) {
	event := TransferEvent{
		BlockNumber: int64(vLog.BlockNumber),
		BlockHash:   vLog.BlockHash.Hex(),
		TxHash:      vLog.TxHash.Hex(),
		LogIndex:    int(vLog.Index),
	}

	if len(vLog.Topics) < 3 {
		return event, fmt.Errorf("事件主题数量不足")
	}

	from := common.HexToAddress(vLog.Topics[1].Hex())
	event.From = strings.ToLower(from.Hex())

	to := common.HexToAddress(vLog.Topics[2].Hex())
	event.To = strings.ToLower(to.Hex())

	if len(vLog.Data) >= 32 {
		value := new(big.Int).SetBytes(vLog.Data)
		event.Value = value
	}

	return event, nil
}

// GetEventChan 获取事件通道
func (c *DualModeClient) GetEventChan() <-chan TransferEvent {
	return c.eventChan
}

// GetCurrentMode 获取当前连接模式
func (c *DualModeClient) GetCurrentMode() ConnectionMode {
	c.modeLock.RLock()
	defer c.modeLock.RUnlock()
	return c.currentMode
}

// setMode 设置连接模式
func (c *DualModeClient) setMode(mode ConnectionMode) {
	c.modeLock.Lock()
	defer c.modeLock.Unlock()
	c.currentMode = mode
}

// IsWSActive 检查 WebSocket 是否激活
func (c *DualModeClient) IsWSActive() bool {
	c.wsClientLock.RLock()
	defer c.wsClientLock.RUnlock()
	return c.wsSubActive
}

// GetCurrentBlockNumber 获取当前区块号 (兼容旧接口)
func (c *DualModeClient) GetCurrentBlockNumber() (int64, error) {
	c.httpClientLock.RLock()
	httpClient := c.httpClient
	c.httpClientLock.RUnlock()

	if httpClient == nil {
		return 0, fmt.Errorf("HTTP 客户端未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	blockNumber, err := httpClient.BlockNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("获取当前区块号失败: %w", err)
	}

	return int64(blockNumber), nil
}

// GetBlockTimestamp 获取区块时间戳 (兼容旧接口)
func (c *DualModeClient) GetBlockTimestamp(blockNumber int64) (time.Time, error) {
	c.httpClientLock.RLock()
	httpClient := c.httpClient
	c.httpClientLock.RUnlock()

	if httpClient == nil {
		return time.Time{}, fmt.Errorf("HTTP 客户端未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	block, err := httpClient.BlockByNumber(ctx, big.NewInt(blockNumber))
	if err != nil {
		return time.Time{}, fmt.Errorf("获取区块信息失败: %w", err)
	}

	return time.Unix(int64(block.Time()), 0), nil
}

// GetTransferEvents 获取 Transfer 事件 (兼容旧接口)
func (c *DualModeClient) GetTransferEvents(fromBlock, toBlock int64) ([]TransferEvent, error) {
	c.httpClientLock.RLock()
	httpClient := c.httpClient
	c.httpClientLock.RUnlock()

	if httpClient == nil {
		return nil, fmt.Errorf("HTTP 客户端未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(fromBlock),
		ToBlock:   big.NewInt(toBlock),
		Addresses: []common.Address{c.contractAddress},
		Topics:    [][]common.Hash{{c.transferEventID}},
	}

	logs, err := httpClient.FilterLogs(ctx, query)
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

// Close 关闭客户端 (兼容旧接口)
func (c *DualModeClient) Close() {
	c.Stop()
}

// GetContractDeploymentBlock 获取合约部署区块号
func (c *DualModeClient) GetContractDeploymentBlock() (int64, error) {
	// 检查合约地址是否有效
	if c.contractAddress == (common.Address{}) {
		return 0, fmt.Errorf("合约地址无效")
	}

	// 从 Etherscan API 获取合约创建区块
	return c.getContractCreationFromEtherscan()
}

// getContractCreationFromEtherscan 从 Etherscan API V2 获取合约创建信息
func (c *DualModeClient) getContractCreationFromEtherscan() (int64, error) {
	apiKey := c.cfg.EtherscanAPIKey
	if apiKey == "" {
		return 0, fmt.Errorf("Etherscan API Key 未配置")
	}

	// V2 API 使用统一的 base URL，通过 chainid 参数指定链
	var chainId string
	switch c.cfg.ChainID {
	case "11155111": // Sepolia
		chainId = "11155111"
	case "84532": // Base Sepolia
		chainId = "84532"
	case "1": // Ethereum Mainnet
		chainId = "1"
	case "8453": // Base Mainnet
		chainId = "8453"
	default:
		return 0, fmt.Errorf("不支持的链 ID: %s，无法使用 Etherscan API", c.cfg.ChainID)
	}

	apiURL := "https://api.etherscan.io/v2/api"
	contractAddr := strings.ToLower(c.contractAddress.Hex())
	url := fmt.Sprintf("%s?chainid=%s&module=contract&action=getcontractcreation&contractaddresses=%s&apikey=%s",
		apiURL, chainId, contractAddr, apiKey)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("Etherscan API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	var etherscanResp struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&etherscanResp); err != nil {
		return 0, fmt.Errorf("解析 Etherscan API 响应失败: %w", err)
	}

	if etherscanResp.Status != "1" {
		var errorMsg string
		if err := json.Unmarshal(etherscanResp.Result, &errorMsg); err == nil {
			return 0, fmt.Errorf("Etherscan API 错误: %s - %s", etherscanResp.Message, errorMsg)
		}
		return 0, fmt.Errorf("Etherscan API 错误: %s", etherscanResp.Message)
	}

	var results []struct {
		ContractAddress string `json:"contractAddress"`
		BlockNumber     string `json:"blockNumber"`
	}
	if err := json.Unmarshal(etherscanResp.Result, &results); err != nil {
		return 0, fmt.Errorf("解析合约创建信息失败: %w", err)
	}

	if len(results) == 0 {
		return 0, fmt.Errorf("Etherscan API 未返回合约创建信息")
	}

	blockNum, err := strconv.ParseInt(results[0].BlockNumber, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("解析区块号失败: %w", err)
	}

	logger.Log.Infof("📡 从 Etherscan 获取到合约 %s 的部署区块: %d", contractAddr, blockNum)
	return blockNum, nil
}
