package controllers

import (
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// BalanceController 余额控制器
type BalanceController struct{}

// NewBalanceController 创建余额控制器
func NewBalanceController() *BalanceController {
	return &BalanceController{}
}

// GetUserBalance 获取用户当前余额
func (bc *BalanceController) GetUserBalance(c *gin.Context) {
	address := strings.ToLower(c.Param("address"))
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户地址不能为空"})
		return
	}

	chainID := c.Query("chain_id")

	query := database.DB.Where("user_address = ?", address)
	if chainID != "" {
		query = query.Where("chain_id = ?", chainID)
	}

	var balances []models.UserBalance
	if err := query.Find(&balances).Error; err != nil {
		logger.Log.Errorf("获取用户余额失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户余额失败"})
		return
	}

	// 获取所有链信息用于查询
	var chains []models.Chain
	chainMap := make(map[string]models.Chain)
	if err := database.DB.Find(&chains).Error; err == nil {
		for _, chain := range chains {
			chainMap[chain.ID] = chain
		}
	}

	// 格式化余额
	var formattedBalances []gin.H
	for _, b := range balances {
		balanceBig, _ := new(big.Int).SetString(b.Balance, 10)
		balanceFormatted := "0"
		if balanceBig != nil {
			balanceFloat := new(big.Float).SetInt(balanceBig)
			divisor := new(big.Float).SetFloat64(1e18)
			balanceFloat.Quo(balanceFloat, divisor)
			balanceFormatted = balanceFloat.Text('f', 6)
		}

		chain := chainMap[b.ChainID]
		formattedBalances = append(formattedBalances, gin.H{
			"chain_name":        chain.Name,
			"chain_id":          chain.ChainID,
			"balance":           b.Balance,
			"balance_formatted": balanceFormatted,
			"last_updated_at":   b.LastUpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_address": address,
		"balances":     formattedBalances,
	})
}

// GetBalanceHistory 获取用户余额变动历史
func (bc *BalanceController) GetBalanceHistory(c *gin.Context) {
	address := strings.ToLower(c.Param("address"))
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户地址不能为空"})
		return
	}

	chainID := c.Query("chain_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	query := database.DB.Where("user_address = ?", address)
	if chainID != "" {
		query = query.Where("chain_id = ?", chainID)
	}

	var total int64
	query.Model(&models.BalanceChange{}).Count(&total)

	var history []models.BalanceChange
	if err := query.Order("timestamp DESC").Offset((page - 1) * limit).Limit(limit).Find(&history).Error; err != nil {
		logger.Log.Errorf("获取余额历史失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额历史失败"})
		return
	}

	// 获取所有链信息
	var chains []models.Chain
	chainMap := make(map[string]models.Chain)
	if err := database.DB.Find(&chains).Error; err == nil {
		for _, chain := range chains {
			chainMap[chain.ID] = chain
		}
	}

	// 格式化历史记录
	var formattedHistory []gin.H
	for _, h := range history {
		changeAmountBig, _ := new(big.Int).SetString(h.ChangeAmount, 10)
		balanceAfterBig, _ := new(big.Int).SetString(h.BalanceAfter, 10)

		changeAmountFormatted := "0"
		balanceAfterFormatted := "0"

		if changeAmountBig != nil {
			balanceFloat := new(big.Float).SetInt(changeAmountBig)
			divisor := new(big.Float).SetFloat64(1e18)
			balanceFloat.Quo(balanceFloat, divisor)
			changeAmountFormatted = balanceFloat.Text('f', 6)
		}

		if balanceAfterBig != nil {
			balanceFloat := new(big.Float).SetInt(balanceAfterBig)
			divisor := new(big.Float).SetFloat64(1e18)
			balanceFloat.Quo(balanceFloat, divisor)
			balanceAfterFormatted = balanceFloat.Text('f', 6)
		}

		chain := chainMap[h.ChainID]
		formattedHistory = append(formattedHistory, gin.H{
			"id":                      h.ID,
			"chain_name":              chain.Name,
			"chain_id":                chain.ChainID,
			"change_type":             h.ChangeType,
			"change_amount":           h.ChangeAmount,
			"change_amount_formatted": changeAmountFormatted,
			"balance_before":          h.BalanceBefore,
			"balance_after":           h.BalanceAfter,
			"balance_after_formatted": balanceAfterFormatted,
			"block_number":            h.BlockNumber,
			"timestamp":               h.Timestamp,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_address": address,
		"history":      formattedHistory,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetAllBalances 获取所有用户余额列表
func (bc *BalanceController) GetAllBalances(c *gin.Context) {
	chainID := c.Query("chain_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	minBalance := c.Query("min_balance")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	query := database.DB
	if chainID != "" {
		query = query.Where("chain_id = ?", chainID)
	}
	if minBalance != "" {
		minBalFloat, _ := strconv.ParseFloat(minBalance, 64)
		if minBalFloat > 0 {
			minBalBig := new(big.Float).Mul(big.NewFloat(minBalFloat), big.NewFloat(1e18))
			minBalInt, _ := minBalBig.Int(nil)
			query = query.Where("balance >= ?", minBalInt.String())
		}
	}

	var total int64
	query.Model(&models.UserBalance{}).Count(&total)

	var balances []models.UserBalance
	if err := query.Order("balance DESC").Offset((page - 1) * limit).Limit(limit).Find(&balances).Error; err != nil {
		logger.Log.Errorf("获取余额列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额列表失败"})
		return
	}

	// 获取所有链信息
	var chains []models.Chain
	chainMap := make(map[string]models.Chain)
	if err := database.DB.Find(&chains).Error; err == nil {
		for _, chain := range chains {
			chainMap[chain.ID] = chain
		}
	}

	// 格式化余额
	var formattedBalances []gin.H
	for _, b := range balances {
		balanceBig, _ := new(big.Int).SetString(b.Balance, 10)
		balanceFormatted := "0"
		if balanceBig != nil {
			balanceFloat := new(big.Float).SetInt(balanceBig)
			divisor := new(big.Float).SetFloat64(1e18)
			balanceFloat.Quo(balanceFloat, divisor)
			balanceFormatted = balanceFloat.Text('f', 6)
		}

		chain := chainMap[b.ChainID]
		formattedBalances = append(formattedBalances, gin.H{
			"user_address":      b.UserAddress,
			"chain_name":        chain.Name,
			"chain_id":          chain.ChainID,
			"balance":           b.Balance,
			"balance_formatted": balanceFormatted,
			"last_updated_at":   b.LastUpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"balances": formattedBalances,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// RebuildUserBalance 重建用户余额
func (bc *BalanceController) RebuildUserBalance(c *gin.Context) {
	address := strings.ToLower(c.Param("address"))
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户地址不能为空"})
		return
	}

	var req struct {
		ChainID string `json:"chain_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	query := database.DB.Where("user_address = ?", address)
	if req.ChainID != "" {
		query = query.Where("chain_id = ?", req.ChainID)
	}

	// 获取该用户的所有余额变动记录
	var changes []models.BalanceChange
	if err := query.Order("timestamp ASC").Find(&changes).Error; err != nil {
		logger.Log.Errorf("获取余额变动记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取余额变动记录失败"})
		return
	}

	if len(changes) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":            "该用户没有余额变动记录",
			"user_address":       address,
			"calculated_balance": "0",
		})
		return
	}

	// 获取最后一条记录的余额
	lastChange := changes[len(changes)-1]
	calculatedBalance := lastChange.BalanceAfter

	// 更新用户余额表
	var userBalance models.UserBalance
	result := database.DB.Where("user_address = ? AND chain_id = ?",
		address, lastChange.ChainID).First(&userBalance)

	if result.Error != nil {
		// 创建新记录
		userBalance = models.UserBalance{
			ID:            "", // 让数据库自动生成
			UserAddress:   address,
			ChainID:       lastChange.ChainID,
			Balance:       calculatedBalance,
			LastUpdatedAt: lastChange.Timestamp,
		}
		database.DB.Create(&userBalance)
	} else {
		// 更新现有记录
		database.DB.Model(&userBalance).Updates(map[string]interface{}{
			"balance":         calculatedBalance,
			"last_updated_at": lastChange.Timestamp,
		})
	}

	balanceBig, _ := new(big.Int).SetString(calculatedBalance, 10)
	balanceFormatted := "0"
	if balanceBig != nil {
		balanceFloat := new(big.Float).SetInt(balanceBig)
		divisor := new(big.Float).SetFloat64(1e18)
		balanceFloat.Quo(balanceFloat, divisor)
		balanceFormatted = balanceFloat.Text('f', 6)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":               "余额重建成功",
		"user_address":          address,
		"chain_id":              lastChange.ChainID,
		"calculated_balance":    calculatedBalance,
		"balance_formatted":     balanceFormatted,
		"last_change_timestamp": lastChange.Timestamp,
	})
}
