package ethereum

import (
	"context"
	"ethclient_service/config"
	"ethclient_service/logger"
	"fmt"
	"math/big"
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
