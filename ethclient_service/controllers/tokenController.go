package controllers

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/models"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
)

type TokenController struct {
	Cfg *config.Config
}

// NewTokenController creates a new TokenController instance
func NewTokenController(cfg *config.Config) *TokenController {
	return &TokenController{Cfg: cfg}
}

// GetBalance handles the request to get a user's token balance
func (c *TokenController) GetBalance(ctx *gin.Context) {
	address := ctx.Query("address")
	if address == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": 40001,
			"msg":  "Address parameter is required",
		})
		return
	}

	// Validate address format
	if !common.IsHexAddress(address) {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": 40002,
			"msg":  "Invalid address format",
		})
		return
	}

	// Query balance from database
	var balance models.Balance
	result := database.DB.Where("address = ?", address).First(&balance)
	if result.Error != nil {
		// If no balance found, return 0
		ctx.JSON(http.StatusOK, gin.H{
			"balance": "0",
			"token":   "MYTOKEN",
		})
		return
	}

	// Return balance
	ctx.JSON(http.StatusOK, gin.H{
		"balance": balance.Balance,
		"token":   "MYTOKEN",
	})
}

// GetTransfers handles the request to get a user's transfer history
func (c *TokenController) GetTransfers(ctx *gin.Context) {
	address := ctx.Query("address")
	if address == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": 40001,
			"msg":  "Address parameter is required",
		})
		return
	}

	// Validate address format
	if !common.IsHexAddress(address) {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code": 40002,
			"msg":  "Invalid address format",
		})
		return
	}

	// Get limit parameter, default to 50
	limitStr := ctx.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100 // Set maximum limit to prevent excessive database queries
	}

	// Query transfers from database
	var transfers []models.Transfer
	result := database.DB.Where("from_addr = ? OR to_addr = ?", address, address).
		Order("block_num desc, log_idx desc").
		Limit(limit).
		Find(&transfers)

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code": 50001,
			"msg":  "Failed to query transfers",
		})
		return
	}

	// Return transfers
	ctx.JSON(http.StatusOK, transfers)
}
