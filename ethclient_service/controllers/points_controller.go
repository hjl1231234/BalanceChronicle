package controllers

import (
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"ethclient_service/services"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PointsController 积分控制器
type PointsController struct {
	pointsCalculator *services.PointsCalculator
}

// NewPointsController 创建积分控制器
func NewPointsController(pointsCalc *services.PointsCalculator) *PointsController {
	return &PointsController{
		pointsCalculator: pointsCalc,
	}
}

// GetUserPoints 获取用户积分
func (pc *PointsController) GetUserPoints(c *gin.Context) {
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

	var points []models.UserPoints
	if err := query.Find(&points).Error; err != nil {
		logger.Log.Errorf("获取用户积分失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户积分失败"})
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

	// 格式化积分
	var formattedPoints []gin.H
	var totalPoints float64

	for _, p := range points {
		chainName := "All Chains"
		chainIDVal := ""
		if p.ChainID != nil {
			if chain, ok := chainMap[*p.ChainID]; ok {
				chainName = chain.Name
				chainIDVal = chain.ChainID
			}
		}

		pointsFloat, _ := strconv.ParseFloat(p.TotalPoints, 64)
		totalPoints += pointsFloat

		formattedPoints = append(formattedPoints, gin.H{
			"chain_name":         chainName,
			"chain_id":           chainIDVal,
			"total_points":       p.TotalPoints,
			"last_calculated_at": p.LastCalculatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_address": address,
		"total_points": strconv.FormatFloat(totalPoints, 'f', 6, 64),
		"details":      formattedPoints,
	})
}

// GetPointsHistory 获取用户积分计算历史
func (pc *PointsController) GetPointsHistory(c *gin.Context) {
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
	query.Model(&models.PointsCalculation{}).Count(&total)

	var history []models.PointsCalculation
	if err := query.Order("start_time DESC").Offset((page - 1) * limit).Limit(limit).Find(&history).Error; err != nil {
		logger.Log.Errorf("获取积分历史失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取积分历史失败"})
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
		chainName := "Unknown"
		chainIDVal := ""
		if chain, ok := chainMap[h.ChainID]; ok {
			chainName = chain.Name
			chainIDVal = chain.ChainID
		}

		balanceBig, _ := new(big.Int).SetString(h.Balance, 10)
		balanceFormatted := "0"
		if balanceBig != nil {
			balanceFloat := new(big.Float).SetInt(balanceBig)
			divisor := new(big.Float).SetFloat64(1e18)
			balanceFloat.Quo(balanceFloat, divisor)
			balanceFormatted = balanceFloat.Text('f', 6)
		}

		formattedHistory = append(formattedHistory, gin.H{
			"id":                h.ID,
			"chain_name":        chainName,
			"chain_id":          chainIDVal,
			"start_time":        h.StartTime,
			"end_time":          h.EndTime,
			"duration_minutes":  h.DurationMinutes,
			"balance":           h.Balance,
			"balance_formatted": balanceFormatted,
			"points_earned":     h.PointsEarned,
			"calculation_time":  h.CalculationTime,
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

// GetPointsLeaderboard 获取积分排行榜
func (pc *PointsController) GetPointsLeaderboard(c *gin.Context) {
	chainID := c.Query("chain_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

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

	var points []models.UserPoints
	if err := query.Order("CAST(total_points AS DECIMAL) DESC").Offset((page - 1) * limit).Limit(limit).Find(&points).Error; err != nil {
		logger.Log.Errorf("获取积分排行榜失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取积分排行榜失败"})
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

	// 格式化排行榜
	var formattedPoints []gin.H
	for i, p := range points {
		chainName := "All Chains"
		chainIDVal := ""
		if p.ChainID != nil {
			if chain, ok := chainMap[*p.ChainID]; ok {
				chainName = chain.Name
				chainIDVal = chain.ChainID
			}
		}

		formattedPoints = append(formattedPoints, gin.H{
			"rank":               (page-1)*limit + i + 1,
			"user_address":       p.UserAddress,
			"chain_name":         chainName,
			"chain_id":           chainIDVal,
			"total_points":       p.TotalPoints,
			"last_calculated_at": p.LastCalculatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"leaderboard": formattedPoints,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
	})
}

// TriggerCalculation 手动触发积分计算
func (pc *PointsController) TriggerCalculation(c *gin.Context) {
	var req struct {
		Address string `json:"address"`
		ChainID string `json:"chain_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	// 异步触发计算
	go pc.pointsCalculator.TriggerCalculation(req.Address, req.ChainID)

	c.JSON(http.StatusOK, gin.H{
		"message":  "积分计算已触发",
		"address":  req.Address,
		"chain_id": req.ChainID,
	})
}

// GetPointsStats 获取积分统计信息
func (pc *PointsController) GetPointsStats(c *gin.Context) {
	chainID := c.Query("chain_id")

	query := database.DB.Model(&models.UserPoints{})
	calcQuery := database.DB.Model(&models.PointsCalculation{})

	if chainID != "" {
		query = query.Where("chain_id = ?", chainID)
		calcQuery = calcQuery.Where("chain_id = ?", chainID)
	}

	// 统计用户数量
	var totalUsers int64
	query.Count(&totalUsers)

	// 获取所有积分并手动求和
	var allPoints []models.UserPoints
	query.Select("total_points").Find(&allPoints)

	var totalPoints float64
	for _, p := range allPoints {
		pointsFloat, _ := strconv.ParseFloat(p.TotalPoints, 64)
		totalPoints += pointsFloat
	}

	// 统计计算次数
	var totalCalculations int64
	calcQuery.Count(&totalCalculations)

	// 统计最近24小时的计算次数
	recentTime := time.Now().Add(-24 * time.Hour)
	var recentCalculations int64
	calcQuery.Where("calculation_time >= ?", recentTime).Count(&recentCalculations)

	c.JSON(http.StatusOK, gin.H{
		"total_users":         totalUsers,
		"total_points":        strconv.FormatFloat(totalPoints, 'f', 6, 64),
		"total_calculations":  totalCalculations,
		"recent_calculations": recentCalculations,
	})
}
