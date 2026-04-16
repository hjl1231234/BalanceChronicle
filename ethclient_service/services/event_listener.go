package services

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/ethereum"
	"ethclient_service/logger"
	"ethclient_service/models"
	"ethclient_service/rabbitmq"
	"ethclient_service/utils"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// EventListener 事件监听器
type EventListener struct {
	clientManager *ethereum.DualModeClientManager
	cfg           *config.Config
	rabbitMQ      *rabbitmq.Client
	stopChan      chan struct{}
	wg            sync.WaitGroup
	isRunning     bool
}

// NewEventListener 创建事件监听器
func NewEventListener(cfg *config.Config) *EventListener {
	return &EventListener{
		clientManager: ethereum.NewDualModeClientManager(),
		cfg:           cfg,
		stopChan:      make(chan struct{}),
		isRunning:     false,
	}
}

// SetRabbitMQClient 设置RabbitMQ客户端
func (el *EventListener) SetRabbitMQClient(client *rabbitmq.Client) {
	el.rabbitMQ = client
}

// Start 启动所有链的事件监听
func (el *EventListener) Start() error {
	if el.isRunning {
		logger.Log.Info("事件监听服务已在运行中")
		return nil
	}

	activeChains := el.cfg.GetActiveChains()
	if len(activeChains) == 0 {
		logger.Log.Warn("没有激活的链配置，事件监听服务未启动")
		return nil
	}

	// 初始化所有链的客户端
	for name, chainConfig := range activeChains {
		// 跳过无效合约地址
		if chainConfig.ContractAddress == "" ||
			chainConfig.ContractAddress == "0x0000000000000000000000000000000000000000" {
			logger.Log.Warnf("链 %s 合约地址无效，跳过监听", name)
			continue
		}

		if err := el.clientManager.AddClient(name, chainConfig, el.cfg.EventListener.PollInterval); err != nil {
			logger.Log.Errorf("初始化链 %s 客户端失败: %v", name, err)
			continue
		}

		// 初始化链记录
		if err := el.initChainRecord(chainConfig); err != nil {
			logger.Log.Errorf("初始化链 %s 记录失败: %v", name, err)
			continue
		}

		// 启动该链的监听
		el.wg.Add(1)
		go el.listenChain(name, chainConfig)
	}

	el.isRunning = true
	logger.Log.Info("✅ 所有链的事件监听服务已启动")
	return nil
}

// Stop 停止所有链的事件监听
func (el *EventListener) Stop() {
	if !el.isRunning {
		return
	}

	close(el.stopChan)
	el.wg.Wait()
	el.clientManager.CloseAll()
	el.isRunning = false
	logger.Log.Info("🛑 所有链的事件监听服务已停止")
}

// initChainRecord 初始化链记录
func (el *EventListener) initChainRecord(chainConfig config.ChainConfig) error {
	var chain models.Chain
	result := database.DB.Where("chain_id = ?", chainConfig.ChainID).First(&chain)

	if result.Error != nil {
		// 创建新记录
		chain = models.Chain{
			ID:                 utils.GenerateID(),
			ChainID:            chainConfig.ChainID,
			Name:               chainConfig.Name,
			RPCURL:             chainConfig.RPCURL,
			ContractAddress:    chainConfig.ContractAddress,
			BlockConfirmations: chainConfig.BlockConfirmations,
			IsActive:           chainConfig.IsActive,
		}
		if err := database.DB.Create(&chain).Error; err != nil {
			return fmt.Errorf("创建链记录失败: %w", err)
		}
	}

	return nil
}

// listenChain 监听单个链的事件
func (el *EventListener) listenChain(chainName string, chainConfig config.ChainConfig) {
	defer el.wg.Done()

	client, ok := el.clientManager.GetClient(chainName)
	if !ok {
		logger.Log.Errorf("链 %s 客户端未找到", chainName)
		return
	}

	// 启动双模式客户端
	if err := client.Start(); err != nil {
		logger.Log.Errorf("启动链 %s 双模式客户端失败: %v", chainName, err)
		return
	}

	logger.Log.Infof("🎧 链 %s 的事件监听已启动 (模式: %s)", chainName, getModeString(client.GetCurrentMode()))

	// 获取事件通道
	eventChan := client.GetEventChan()

	for {
		select {
		case <-el.stopChan:
			logger.Log.Infof("🛑 链 %s 的事件监听已停止", chainName)
			return
		case event := <-eventChan:
			// 处理 WebSocket 实时事件
			if err := el.processRealtimeEvent(chainName, chainConfig, event); err != nil {
				logger.Log.Errorf("处理链 %s 实时事件时出错: %v", chainName, err)
			}
		}
	}
}

// getModeString 获取模式字符串
func getModeString(mode ethereum.ConnectionMode) string {
	if mode == ethereum.ModeWS {
		return "WebSocket"
	}
	return "HTTP"
}

// processRealtimeEvent 处理实时事件 (来自 WebSocket)
func (el *EventListener) processRealtimeEvent(chainName string, chainConfig config.ChainConfig, event ethereum.TransferEvent) error {
	// 获取链记录
	var chain models.Chain
	if err := database.DB.Where("chain_id = ?", chainConfig.ChainID).First(&chain).Error; err != nil {
		return fmt.Errorf("获取链记录失败: %w", err)
	}

	// 检查确认数
	client, ok := el.clientManager.GetClient(chainName)
	if !ok {
		return fmt.Errorf("客户端未找到")
	}

	currentBlock, err := client.GetCurrentBlockNumber()
	if err != nil {
		return fmt.Errorf("获取当前区块号失败: %w", err)
	}

	// 计算确认数
	confirmations := currentBlock - event.BlockNumber
	if confirmations < int64(chainConfig.BlockConfirmations) {
		// 未达到确认数，跳过
		return nil
	}

	// 处理事件
	if err := el.processEvent(&chain, event); err != nil {
		return fmt.Errorf("处理事件失败: %w", err)
	}

	logger.Log.Debugf("✅ 链 %s: 实时处理事件 %s:%d", chainName, event.TxHash, event.LogIndex)
	return nil
}

// processChainEvents 处理链事件 (HTTP 轮询模式，用于历史同步)
func (el *EventListener) processChainEvents(chainName string, chainConfig config.ChainConfig, client *ethereum.DualModeClient) error {
	// 获取链记录
	var chain models.Chain
	if err := database.DB.Where("chain_id = ?", chainConfig.ChainID).First(&chain).Error; err != nil {
		return fmt.Errorf("获取链记录失败: %w", err)
	}

	// 获取同步状态
	var syncState models.SyncState
	result := database.DB.Where("chain_id = ?", chain.ID).First(&syncState)

	// 获取当前区块号
	currentBlock, err := client.GetCurrentBlockNumber()
	if err != nil {
		return fmt.Errorf("获取当前区块号失败: %w", err)
	}

	// 计算安全区块（当前区块 - 确认数）
	safeBlock := currentBlock - int64(chainConfig.BlockConfirmations)

	// 确定起始区块
	var fromBlock int64
	if result.Error != nil {
		// 首次同步，从合约部署区块开始
		if chainConfig.ContractBlock > 0 {
			// 配置了合约部署区块，直接使用
			fromBlock = chainConfig.ContractBlock
			logger.Log.Infof("📍 链 %s: 使用配置的合约部署区块 %d", chainName, fromBlock)
		} else {
			// 未配置，尝试自动检测合约部署区块
			logger.Log.Infof("🔍 链 %s: 未配置合约部署区块，尝试自动检测...", chainName)
			deploymentBlock, err := client.GetContractDeploymentBlock()
			if err != nil {
				// 自动检测失败，从创世区块开始
				logger.Log.Warnf("⚠️ 链 %s: 自动检测合约部署区块失败: %v，将从创世区块开始", chainName, err)
				fromBlock = 0
			} else {
				fromBlock = deploymentBlock
				logger.Log.Infof("✅ 链 %s: 自动检测到合约部署区块 %d", chainName, fromBlock)
			}
		}

		syncState = models.SyncState{
			ID:              utils.GenerateID(),
			ChainID:         chain.ID,
			LastSyncedBlock: fromBlock - 1,
			IsSyncing:       false,
		}
		database.DB.Create(&syncState)
		logger.Log.Infof("🆕 链 %s: 首次同步，从区块 %d 开始", chainName, fromBlock)
	} else {
		fromBlock = syncState.LastSyncedBlock + 1
	}

	// 如果没有新区块，跳过
	if fromBlock > safeBlock || syncState.IsSyncing {
		return nil
	}

	// 标记为正在同步
	database.DB.Model(&syncState).Update("is_syncing", true)

	// 计算本次处理的区块范围
	toBlock := fromBlock + int64(el.cfg.EventListener.BatchSize)
	if toBlock > safeBlock {
		toBlock = safeBlock
	}

	logger.Log.Infof("📦 链 %s: 处理区块 %d 到 %d (当前: %d, 安全: %d)",
		chainName, fromBlock, toBlock, currentBlock, safeBlock)

	// 查询 Transfer 事件
	events, err := client.GetTransferEvents(fromBlock, toBlock)
	if err != nil {
		database.DB.Model(&syncState).Update("is_syncing", false)
		return fmt.Errorf("查询事件失败: %w", err)
	}

	// 处理事件
	for _, event := range events {
		if err := el.processEvent(&chain, event); err != nil {
			logger.Log.Warnf("处理事件失败: %v", err)
			continue
		}
	}

	// 更新同步状态
	database.DB.Model(&syncState).Updates(map[string]interface{}{
		"last_synced_block": toBlock,
		"is_syncing":        false,
	})

	logger.Log.Infof("✅ 链 %s: 已处理 %d 个事件，同步到区块 %d", chainName, len(events), toBlock)
	return nil
}

// processEvent 处理单个事件 - 发送到MQ
func (el *EventListener) processEvent(chain *models.Chain, event ethereum.TransferEvent) error {
	// 检查事件是否已存在（幂等性检查）
	var existingEvent models.Event
	result := database.DB.Where("chain_id = ? AND transaction_hash = ? AND log_index = ?",
		chain.ID, event.TxHash, event.LogIndex).First(&existingEvent)

	if result.Error == nil {
		// 已存在，跳过
		return nil
	}

	// 使用当前时间作为时间戳
	timestamp := time.Now()

	// 发送到RabbitMQ
	if el.rabbitMQ != nil && el.rabbitMQ.IsRunning() {
		msg := &rabbitmq.TransferEventMessage{
			ChainID:         chain.ChainID,
			ChainName:       chain.Name,
			BlockNumber:     event.BlockNumber,
			BlockHash:       event.BlockHash,
			TransactionHash: event.TxHash,
			LogIndex:        event.LogIndex,
			From:            event.From,
			To:              event.To,
			Value:           event.Value.String(),
			Timestamp:       timestamp.Unix(),
		}

		if err := el.rabbitMQ.PublishEvent(msg); err != nil {
			logger.Log.Errorf("发送事件到MQ失败: %v", err)
			// MQ发送失败不阻塞，继续保存到数据库（降级方案）
		} else {
			logger.Log.Debugf("✅ 事件已发送到MQ: %s:%d", event.TxHash, event.LogIndex)
		}
	}

	// 同时保存到Event表（用于幂等性检查）
	eventRecord := models.Event{
		ID:              utils.GenerateID(),
		ChainID:         chain.ID,
		BlockNumber:     event.BlockNumber,
		BlockHash:       event.BlockHash,
		TransactionHash: event.TxHash,
		LogIndex:        event.LogIndex,
		EventType:       "Transfer",
		FromAddress:     &event.From,
		ToAddress:       &event.To,
		Amount:          event.Value.String(),
		IsProcessed:     false,
		ConfirmedAt:     &timestamp,
		CreatedAt:       time.Now(),
	}

	if err := database.DB.Create(&eventRecord).Error; err != nil {
		return fmt.Errorf("创建事件记录失败: %w", err)
	}

	return nil
}

// processBalanceChanges 处理余额变动
func (el *EventListener) processBalanceChanges(event *models.Event, chain *models.Chain, transferEvent ethereum.TransferEvent, timestamp time.Time) error {
	zeroAddress := "0x0000000000000000000000000000000000000000"

	// 处理转出方（如果不是零地址）
	if !strings.EqualFold(transferEvent.From, zeroAddress) {
		if err := el.updateUserBalance(event.ID, chain.ID, transferEvent.From,
			new(big.Int).Neg(transferEvent.Value), "transfer_out", transferEvent.BlockNumber, timestamp); err != nil {
			return err
		}
	} else {
		// Mint 事件 - 处理接收方
		if err := el.updateUserBalance(event.ID, chain.ID, transferEvent.To,
			transferEvent.Value, "mint", transferEvent.BlockNumber, timestamp); err != nil {
			return err
		}
	}

	// 处理转入方（如果不是零地址且不是 Mint 事件）
	if !strings.EqualFold(transferEvent.To, zeroAddress) && !strings.EqualFold(transferEvent.From, zeroAddress) {
		if err := el.updateUserBalance(event.ID, chain.ID, transferEvent.To,
			transferEvent.Value, "transfer_in", transferEvent.BlockNumber, timestamp); err != nil {
			return err
		}
	} else if strings.EqualFold(transferEvent.To, zeroAddress) {
		// Burn 事件
		if err := el.updateUserBalance(event.ID, chain.ID, transferEvent.From,
			new(big.Int).Neg(transferEvent.Value), "burn", transferEvent.BlockNumber, timestamp); err != nil {
			return err
		}
	}

	return nil
}

// updateUserBalance 更新用户余额
func (el *EventListener) updateUserBalance(eventID, chainID, userAddress string,
	changeAmount *big.Int, changeType string, blockNumber int64, timestamp time.Time) error {

	// 标准化地址
	userAddress = strings.ToLower(userAddress)

	// 获取或创建用户余额记录
	var userBalance models.UserBalance
	result := database.DB.Where("user_address = ? AND chain_id = ?",
		userAddress, chainID).First(&userBalance)

	balanceBefore := big.NewInt(0)
	if result.Error == nil {
		balanceBefore, _ = new(big.Int).SetString(userBalance.Balance, 10)
	}

	balanceAfter := new(big.Int).Add(balanceBefore, changeAmount)

	// 创建余额变动记录
	balanceChange := models.BalanceChange{
		ID:            utils.GenerateID(),
		EventID:       eventID,
		UserAddress:   userAddress,
		ChainID:       chainID,
		ChangeAmount:  changeAmount.String(),
		BalanceBefore: balanceBefore.String(),
		BalanceAfter:  balanceAfter.String(),
		ChangeType:    changeType,
		BlockNumber:   blockNumber,
		Timestamp:     timestamp,
		CreatedAt:     time.Now(),
	}

	if err := database.DB.Create(&balanceChange).Error; err != nil {
		return fmt.Errorf("创建余额变动记录失败: %w", err)
	}

	// 更新用户总余额
	if result.Error != nil {
		// 创建新记录
		userBalance = models.UserBalance{
			ID:            utils.GenerateID(),
			UserAddress:   userAddress,
			ChainID:       chainID,
			Balance:       balanceAfter.String(),
			LastUpdatedAt: timestamp,
			CreatedAt:     time.Now(),
		}
		if err := database.DB.Create(&userBalance).Error; err != nil {
			return fmt.Errorf("创建用户余额记录失败: %w", err)
		}
	} else {
		// 更新现有记录
		database.DB.Model(&userBalance).Updates(map[string]interface{}{
			"balance":         balanceAfter.String(),
			"last_updated_at": timestamp,
		})
	}

	logger.Log.Infof("💰 用户 %s 余额变动: %s (%s), 当前余额: %s",
		userAddress, changeAmount.String(), changeType, balanceAfter.String())

	return nil
}
