package services

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"ethclient_service/utils"
	"fmt"
	"math/big"
	"sync"
	"time"

	"gorm.io/gorm"
)

// PointsCalculator 积分计算器
type PointsCalculator struct {
	cfg       *config.Config
	stopChan  chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	ticker    *time.Ticker
}

// NewPointsCalculator 创建积分计算器
func NewPointsCalculator(cfg *config.Config) *PointsCalculator {
	return &PointsCalculator{
		cfg:       cfg,
		stopChan:  make(chan struct{}),
		isRunning: false,
	}
}

// Start 启动积分计算定时任务
func (pc *PointsCalculator) Start() {
	if pc.isRunning {
		logger.Log.Info("积分计算服务已在运行中")
		return
	}

	pc.isRunning = true
	pc.ticker = time.NewTicker(time.Duration(pc.cfg.Points.CalculationInterval) * time.Millisecond)

	// 立即执行一次
	go pc.calculateAllPoints()

	// 定时执行
	pc.wg.Add(1)
	go func() {
		defer pc.wg.Done()
		for {
			select {
			case <-pc.stopChan:
				return
			case <-pc.ticker.C:
				pc.calculateAllPoints()
			}
		}
	}()

	logger.Log.Infof("✅ 积分计算服务已启动，每 %d 毫秒计算一次", pc.cfg.Points.CalculationInterval)
}

// Stop 停止积分计算服务
func (pc *PointsCalculator) Stop() {
	if !pc.isRunning {
		return
	}

	pc.ticker.Stop()
	close(pc.stopChan)
	pc.wg.Wait()
	pc.isRunning = false
	logger.Log.Info("🛑 积分计算服务已停止")
}

// calculateAllPoints 计算所有用户的积分
func (pc *PointsCalculator) calculateAllPoints() {
	logger.Log.Info("🧮 开始计算积分...")
	startTime := time.Now()

	// 获取所有有余额的用户
	var userBalances []models.UserBalance
	if err := database.DB.Where("balance != ?", "0").Find(&userBalances).Error; err != nil {
		logger.Log.Errorf("获取用户余额失败: %v", err)
		return
	}

	logger.Log.Infof("📊 需要计算积分的用户数量: %d", len(userBalances))

	// 限制单次计算的最大时长，避免长时间运行
	maxCalculationTime := time.Duration(pc.cfg.Points.CalculationInterval) * time.Millisecond
	if maxCalculationTime < time.Minute {
		maxCalculationTime = time.Minute // 至少1分钟
	}
	deadline := startTime.Add(maxCalculationTime)

	successCount := 0
	errorCount := 0

	for _, userBalance := range userBalances {
		// 检查是否超时
		if time.Now().After(deadline) {
			logger.Log.Warnf("⏰ 积分计算接近超时，已处理 %d/%d 用户，剩余将在下次计算", successCount, len(userBalances))
			break
		}

		if err := pc.calculateUserPointsWithTx(userBalance.UserAddress, userBalance.ChainID); err != nil {
			logger.Log.Errorf("计算用户 %s 积分失败: %v", userBalance.UserAddress, err)
			errorCount++
		} else {
			successCount++
		}
	}

	duration := time.Since(startTime)
	logger.Log.Infof("✅ 积分计算完成，成功: %d, 失败: %d, 耗时 %v", successCount, errorCount, duration)
}

// calculateUserPointsWithTx 使用事务计算单个用户的积分
func (pc *PointsCalculator) calculateUserPointsWithTx(userAddress, chainID string) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		return pc.calculateUserPointsTx(tx, userAddress, chainID)
	})
}

// calculateUserPointsTx 在事务中计算单个用户的积分
func (pc *PointsCalculator) calculateUserPointsTx(tx *gorm.DB, userAddress, chainID string) error {
	// 获取该用户最后一次积分计算的时间（使用FOR UPDATE锁定，防止并发）
	var lastCalculation models.PointsCalculation
	result := tx.Where("user_address = ? AND chain_id = ?",
		userAddress, chainID).Order("end_time DESC").First(&lastCalculation)

	// 确定本次计算的起始时间
	var calculationStartTime time.Time
	if result.Error == nil {
		calculationStartTime = lastCalculation.EndTime
	} else {
		// 获取第一次余额变动的时间
		var firstChange models.BalanceChange
		if err := tx.Where("user_address = ? AND chain_id = ?",
			userAddress, chainID).Order("timestamp ASC").First(&firstChange).Error; err != nil {
			// 没有余额变动记录，跳过
			return nil
		}
		calculationStartTime = firstChange.Timestamp
	}

	now := time.Now()

	// 限制单次计算的最大时间范围（例如最多计算24小时），避免长时间停机后单次计算过多
	maxCalculationPeriod := 24 * time.Hour
	if now.Sub(calculationStartTime) > maxCalculationPeriod {
		now = calculationStartTime.Add(maxCalculationPeriod)
		logger.Log.Debugf("用户 %s 计算时间范围过大，限制为24小时", userAddress)
	}

	// 如果计算时间太短（小于1分钟），跳过
	if now.Sub(calculationStartTime) < time.Minute {
		return nil
	}

	// 获取该时间段内的所有余额变动
	var balanceChanges []models.BalanceChange
	if err := tx.Where("user_address = ? AND chain_id = ? AND timestamp >= ? AND timestamp <= ?",
		userAddress, chainID, calculationStartTime, now).Order("timestamp ASC").Find(&balanceChanges).Error; err != nil {
		return fmt.Errorf("获取余额变动记录失败: %w", err)
	}

	if len(balanceChanges) == 0 && result.Error != nil {
		// 没有余额变动记录且从未计算过积分，跳过
		return nil
	}

	// 获取初始余额
	initialBalance := "0"
	if result.Error == nil {
		initialBalance = lastCalculation.Balance
	}

	// 构建余额时间段
	balancePeriods := pc.buildBalancePeriods(balanceChanges, calculationStartTime, now, initialBalance)

	// 计算每个时间段的积分
	var totalPointsEarned float64

	for _, period := range balancePeriods {
		pointsEarned := pc.calculatePeriodPoints(period.Balance, period.DurationMinutes)

		if pointsEarned > 0 {
			// 保存积分计算记录
			pointsCalc := models.PointsCalculation{
				ID:              utils.GenerateID(),
				UserAddress:     userAddress,
				ChainID:         chainID,
				StartTime:       period.StartTime,
				EndTime:         period.EndTime,
				Balance:         period.Balance,
				DurationMinutes: period.DurationMinutes,
				PointsEarned:    fmt.Sprintf("%.6f", pointsEarned),
				CalculationTime: time.Now(),
			}

			if err := tx.Create(&pointsCalc).Error; err != nil {
				return fmt.Errorf("保存积分计算记录失败: %w", err)
			}

			totalPointsEarned += pointsEarned
		}
	}

	// 更新用户总积分
	if totalPointsEarned > 0 {
		if err := pc.updateUserTotalPointsTx(tx, userAddress, chainID, totalPointsEarned); err != nil {
			return fmt.Errorf("更新用户总积分失败: %w", err)
		}

		logger.Log.Infof("⭐ 用户 %s 获得积分: %.6f", userAddress, totalPointsEarned)
	}

	return nil
}

// calculateUserPoints 计算单个用户的积分（兼容旧接口，使用默认事务）
func (pc *PointsCalculator) calculateUserPoints(userAddress, chainID string) error {
	return pc.calculateUserPointsWithTx(userAddress, chainID)
}

// BalancePeriod 余额时间段
type BalancePeriod struct {
	StartTime       time.Time
	EndTime         time.Time
	Balance         string
	DurationMinutes float64
}

// buildBalancePeriods 构建余额时间段
func (pc *PointsCalculator) buildBalancePeriods(balanceChanges []models.BalanceChange,
	startTime, endTime time.Time, initialBalance string) []BalancePeriod {

	var periods []BalancePeriod
	currentTime := startTime
	currentBalance := initialBalance

	for _, change := range balanceChanges {
		changeTime := change.Timestamp

		// 如果当前余额保持了一段时间
		if changeTime.After(currentTime) {
			durationMs := changeTime.Sub(currentTime).Milliseconds()
			durationMinutes := float64(durationMs) / (1000 * 60)

			if durationMinutes > 0 {
				periods = append(periods, BalancePeriod{
					StartTime:       currentTime,
					EndTime:         changeTime,
					Balance:         currentBalance,
					DurationMinutes: durationMinutes,
				})
			}
		}

		currentTime = changeTime
		currentBalance = change.BalanceAfter
	}

	// 处理最后一个时间段到当前时间
	if endTime.After(currentTime) {
		durationMs := endTime.Sub(currentTime).Milliseconds()
		durationMinutes := float64(durationMs) / (1000 * 60)

		if durationMinutes > 0 {
			periods = append(periods, BalancePeriod{
				StartTime:       currentTime,
				EndTime:         endTime,
				Balance:         currentBalance,
				DurationMinutes: durationMinutes,
			})
		}
	}

	return periods
}

// calculatePeriodPoints 计算单个时间段的积分
func (pc *PointsCalculator) calculatePeriodPoints(balance string, durationMinutes float64) float64 {
	// 将余额转换为可计算的数值（假设代币有18位小数）
	balanceBig, ok := new(big.Int).SetString(balance, 10)
	if !ok {
		return 0
	}

	// 转换为标准单位（除以 10^18）
	balanceFloat := new(big.Float).SetInt(balanceBig)
	divisor := new(big.Float).SetFloat64(1e18)
	balanceFloat.Quo(balanceFloat, divisor)

	balanceNum, _ := balanceFloat.Float64()

	// 积分 = 余额 * 0.05 * (持续时间分钟数 / 60)
	points := balanceNum * pc.cfg.Points.Rate * (durationMinutes / 60)

	// 保留6位小数
	return float64(int64(points*1e6)) / 1e6
}

// updateUserTotalPointsTx 在事务中更新用户总积分
func (pc *PointsCalculator) updateUserTotalPointsTx(tx *gorm.DB, userAddress, chainID string, pointsEarned float64) error {
	// 查找现有的总积分记录（使用FOR UPDATE锁定）
	var userPoints models.UserPoints
	result := tx.Where("user_address = ? AND chain_id = ?",
		userAddress, chainID).First(&userPoints)

	if result.Error != nil {
		// 创建新记录
		userPoints = models.UserPoints{
			ID:               utils.GenerateID(),
			UserAddress:      userAddress,
			ChainID:          &chainID,
			TotalPoints:      fmt.Sprintf("%.6f", pointsEarned),
			LastCalculatedAt: time.Now(),
		}
		return tx.Create(&userPoints).Error
	}

	// 更新现有记录
	currentTotal, _ := new(big.Float).SetString(userPoints.TotalPoints)
	earned := new(big.Float).SetFloat64(pointsEarned)
	newTotal := new(big.Float).Add(currentTotal, earned)
	newTotalStr := newTotal.Text('f', 6)

	return tx.Model(&userPoints).Updates(map[string]interface{}{
		"total_points":       newTotalStr,
		"last_calculated_at": time.Now(),
	}).Error
}

// TriggerCalculation 手动触发积分计算
func (pc *PointsCalculator) TriggerCalculation(userAddress, chainID string) {
	logger.Log.Info("🚀 手动触发积分计算...")

	if userAddress != "" && chainID != "" {
		if err := pc.calculateUserPointsWithTx(userAddress, chainID); err != nil {
			logger.Log.Errorf("计算用户积分失败: %v", err)
		}
	} else {
		pc.calculateAllPoints()
	}
}
