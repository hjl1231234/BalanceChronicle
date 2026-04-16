package services

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"ethclient_service/rabbitmq"
	"ethclient_service/utils"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// EventConsumer MQ事件消费者
type EventConsumer struct {
	cfg       *config.Config
	rabbitMQ  *rabbitmq.Client
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isRunning bool
}

// NewEventConsumer 创建事件消费者
func NewEventConsumer(cfg *config.Config) *EventConsumer {
	return &EventConsumer{
		cfg:       cfg,
		stopChan:  make(chan struct{}),
		isRunning: false,
	}
}

// SetRabbitMQClient 设置RabbitMQ客户端
func (ec *EventConsumer) SetRabbitMQClient(client *rabbitmq.Client) {
	ec.rabbitMQ = client
}

// Start 启动消费者
func (ec *EventConsumer) Start() error {
	if ec.isRunning {
		logger.Log.Info("事件消费者已在运行中")
		return nil
	}

	if ec.rabbitMQ == nil || !ec.rabbitMQ.IsRunning() {
		return fmt.Errorf("RabbitMQ客户端未初始化")
	}

	ec.wg.Add(1)
	go ec.consume()

	ec.isRunning = true
	logger.Log.Info("✅ 事件消费者已启动")
	return nil
}

// Stop 停止消费者
func (ec *EventConsumer) Stop() {
	if !ec.isRunning {
		return
	}

	close(ec.stopChan)
	ec.wg.Wait()
	ec.isRunning = false
	logger.Log.Info("🛑 事件消费者已停止")
}

// consume 消费消息
func (ec *EventConsumer) consume() {
	defer ec.wg.Done()

	err := ec.rabbitMQ.Consume(func(msg *rabbitmq.TransferEventMessage) error {
		return ec.processMessage(msg)
	})

	if err != nil {
		logger.Log.Errorf("启动消费失败: %v", err)
	}

	// 等待停止信号
	<-ec.stopChan
}

// processMessage 处理MQ消息
func (ec *EventConsumer) processMessage(msg *rabbitmq.TransferEventMessage) error {
	// 获取链记录
	var chain models.Chain
	if err := database.DB.Where("chain_id = ?", msg.ChainID).First(&chain).Error; err != nil {
		return fmt.Errorf("获取链记录失败: %w", err)
	}

	// 检查事件是否已处理（幂等性）
	var existingEvent models.Event
	result := database.DB.Where("chain_id = ? AND transaction_hash = ? AND log_index = ?",
		chain.ID, msg.TransactionHash, msg.LogIndex).First(&existingEvent)

	if result.Error == nil && existingEvent.IsProcessed {
		// 已处理，跳过
		logger.Log.Debugf("事件已处理，跳过: %s:%d", msg.TransactionHash, msg.LogIndex)
		return nil
	}

	// 使用事务处理
	return database.DB.Transaction(func(tx *gorm.DB) error {
		timestamp := time.Unix(msg.Timestamp, 0)

		// 创建或更新Event记录
		var eventRecord models.Event
		if result.Error != nil {
			// 新事件，创建记录
			eventRecord = models.Event{
				ID:              utils.GenerateID(),
				ChainID:         chain.ID,
				BlockNumber:     msg.BlockNumber,
				BlockHash:       msg.BlockHash,
				TransactionHash: msg.TransactionHash,
				LogIndex:        msg.LogIndex,
				EventType:       "Transfer",
				FromAddress:     &msg.From,
				ToAddress:       &msg.To,
				Amount:          msg.Value,
				IsProcessed:     true,
				ConfirmedAt:     &timestamp,
				CreatedAt:       time.Now(),
			}
			if err := tx.Create(&eventRecord).Error; err != nil {
				return fmt.Errorf("创建事件记录失败: %w", err)
			}
		} else {
			// 已存在但未处理，更新状态
			eventRecord = existingEvent
			if err := tx.Model(&eventRecord).Update("is_processed", true).Error; err != nil {
				return fmt.Errorf("更新事件状态失败: %w", err)
			}
		}

		// 处理余额变动
		value, ok := new(big.Int).SetString(msg.Value, 10)
		if !ok {
			return fmt.Errorf("解析金额失败: %s", msg.Value)
		}

		if err := ec.processBalanceChangesTx(tx, &eventRecord, &chain, msg.From, msg.To, value, msg.BlockNumber, timestamp); err != nil {
			return fmt.Errorf("处理余额变动失败: %w", err)
		}

		logger.Log.Infof("✅ 事件处理完成: %s:%d", msg.TransactionHash, msg.LogIndex)
		return nil
	})
}

// processBalanceChangesTx 在事务中处理余额变动
func (ec *EventConsumer) processBalanceChangesTx(tx *gorm.DB, event *models.Event, chain *models.Chain,
	from, to string, value *big.Int, blockNumber int64, timestamp time.Time) error {

	zeroAddress := "0x0000000000000000000000000000000000000000"

	// 处理转出方（如果不是零地址）
	if !strings.EqualFold(from, zeroAddress) {
		if err := ec.updateUserBalanceTx(tx, event.ID, chain.ID, from,
			new(big.Int).Neg(value), "transfer_out", blockNumber, timestamp); err != nil {
			return err
		}
	} else {
		// Mint 事件 - 处理接收方
		if err := ec.updateUserBalanceTx(tx, event.ID, chain.ID, to,
			value, "mint", blockNumber, timestamp); err != nil {
			return err
		}
	}

	// 处理转入方（如果不是零地址且不是 Mint 事件）
	if !strings.EqualFold(to, zeroAddress) && !strings.EqualFold(from, zeroAddress) {
		if err := ec.updateUserBalanceTx(tx, event.ID, chain.ID, to,
			value, "transfer_in", blockNumber, timestamp); err != nil {
			return err
		}
	} else if strings.EqualFold(to, zeroAddress) {
		// Burn 事件
		if err := ec.updateUserBalanceTx(tx, event.ID, chain.ID, from,
			new(big.Int).Neg(value), "burn", blockNumber, timestamp); err != nil {
			return err
		}
	}

	return nil
}

// updateUserBalanceTx 在事务中更新用户余额
func (ec *EventConsumer) updateUserBalanceTx(tx *gorm.DB, eventID, chainID, userAddress string,
	changeAmount *big.Int, changeType string, blockNumber int64, timestamp time.Time) error {

	// 标准化地址
	userAddress = strings.ToLower(userAddress)

	// 获取或创建用户余额记录（使用FOR UPDATE锁定）
	var userBalance models.UserBalance
	result := tx.Where("user_address = ? AND chain_id = ?", userAddress, chainID).First(&userBalance)

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

	if err := tx.Create(&balanceChange).Error; err != nil {
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
		if err := tx.Create(&userBalance).Error; err != nil {
			return fmt.Errorf("创建用户余额记录失败: %w", err)
		}
	} else {
		// 更新现有记录
		if err := tx.Model(&userBalance).Updates(map[string]interface{}{
			"balance":         balanceAfter.String(),
			"last_updated_at": timestamp,
		}).Error; err != nil {
			return fmt.Errorf("更新用户余额失败: %w", err)
		}
	}

	return nil
}
