package controllers

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChainController 链控制器
type ChainController struct {
	cfg *config.Config
}

// NewChainController 创建链控制器
func NewChainController(cfg *config.Config) *ChainController {
	return &ChainController{cfg: cfg}
}

// GetAllChains 获取所有链的信息
func (cc *ChainController) GetAllChains(c *gin.Context) {
	var chains []models.Chain
	if err := database.DB.Find(&chains).Error; err != nil {
		logger.Log.Errorf("获取链信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取链信息失败"})
		return
	}

	// 获取所有同步状态
	var syncStates []models.SyncState
	syncStateMap := make(map[string]models.SyncState)
	if err := database.DB.Find(&syncStates).Error; err == nil {
		for _, ss := range syncStates {
			syncStateMap[ss.ChainID] = ss
		}
	}

	var formattedChains []gin.H
	for _, chain := range chains {
		syncState := gin.H{
			"last_synced_block": 0,
			"is_syncing":        false,
			"updated_at":        nil,
		}

		if ss, ok := syncStateMap[chain.ID]; ok {
			syncState = gin.H{
				"last_synced_block": ss.LastSyncedBlock,
				"is_syncing":        ss.IsSyncing,
				"updated_at":        ss.UpdatedAt,
			}
		}

		formattedChains = append(formattedChains, gin.H{
			"id":                  chain.ID,
			"chain_id":            chain.ChainID,
			"name":                chain.Name,
			"contract_address":    chain.ContractAddress,
			"block_confirmations": chain.BlockConfirmations,
			"is_active":           chain.IsActive,
			"sync_state":          syncState,
			"created_at":          chain.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"chains": formattedChains})
}

// GetChainByID 获取单个链的信息
func (cc *ChainController) GetChainByID(c *gin.Context) {
	id := c.Param("id")

	var chain models.Chain
	if err := database.DB.Where("id = ?", id).First(&chain).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "链未找到"})
		return
	}

	// 获取同步状态
	var syncState models.SyncState
	syncStateData := gin.H{
		"last_synced_block": 0,
		"is_syncing":        false,
		"updated_at":        nil,
	}

	if err := database.DB.Where("chain_id = ?", id).First(&syncState).Error; err == nil {
		syncStateData = gin.H{
			"last_synced_block": syncState.LastSyncedBlock,
			"is_syncing":        syncState.IsSyncing,
			"updated_at":        syncState.UpdatedAt,
		}
	}

	// 尝试获取当前区块高度
	currentBlock := "0"
	// 这里可以从客户端获取，简化处理

	c.JSON(http.StatusOK, gin.H{
		"id":                  chain.ID,
		"chain_id":            chain.ChainID,
		"name":                chain.Name,
		"contract_address":    chain.ContractAddress,
		"block_confirmations": chain.BlockConfirmations,
		"is_active":           chain.IsActive,
		"current_block":       currentBlock,
		"sync_state":          syncStateData,
		"created_at":          chain.CreatedAt,
		"updated_at":          chain.UpdatedAt,
	})
}

// GetChainConfigs 获取链配置
func (cc *ChainController) GetChainConfigs(c *gin.Context) {
	var safeConfigs []gin.H

	for name, chainConfig := range cc.cfg.Chains {
		safeConfigs = append(safeConfigs, gin.H{
			"name":                name,
			"chain_id":            chainConfig.ChainID,
			"contract_address":    chainConfig.ContractAddress,
			"block_confirmations": chainConfig.BlockConfirmations,
			"is_active":           chainConfig.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{"configs": safeConfigs})
}

// SyncChain 手动同步链
func (cc *ChainController) SyncChain(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		FromBlock int64 `json:"from_block"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	var chain models.Chain
	if err := database.DB.Where("id = ?", id).First(&chain).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "链未找到"})
		return
	}

	// 更新同步状态
	var syncState models.SyncState
	result := database.DB.Where("chain_id = ?", id).First(&syncState)

	if result.Error != nil {
		// 创建新记录 - 使用链ID作为ID
		syncState = models.SyncState{
			ID:              id + "_sync",
			ChainID:         id,
			LastSyncedBlock: req.FromBlock - 1,
			IsSyncing:       false,
		}
		database.DB.Create(&syncState)
	} else {
		// 更新现有记录
		database.DB.Model(&syncState).Updates(map[string]interface{}{
			"last_synced_block": req.FromBlock - 1,
			"is_syncing":        false,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "链同步状态已重置",
		"chain_id":   id,
		"from_block": req.FromBlock,
	})
}

// GetPointsRate 获取积分计算比率
func (cc *ChainController) GetPointsRate(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"rate":                 cc.cfg.Points.Rate,
		"calculation_interval": cc.cfg.Points.CalculationInterval,
	})
}

// UpdatePointsRate 更新积分计算比率
func (cc *ChainController) UpdatePointsRate(c *gin.Context) {
	var req struct {
		Rate float64 `json:"rate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if req.Rate <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "积分比率必须大于0"})
		return
	}

	cc.cfg.Points.Rate = req.Rate

	c.JSON(http.StatusOK, gin.H{
		"message": "积分比率已更新",
		"rate":    req.Rate,
	})
}
