package routes

import (
	"ethclient_service/config"
	"ethclient_service/controllers"
	"ethclient_service/services"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置路由
func SetupRoutes(router *gin.Engine, cfg *config.Config, pointsCalc *services.PointsCalculator) {
	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "ok",
			"timestamp": "now",
		})
	})

	// 创建控制器
	balanceController := controllers.NewBalanceController()
	pointsController := controllers.NewPointsController(pointsCalc)
	chainController := controllers.NewChainController(cfg)

	// API 路由组
	api := router.Group("/api")
	{
		// 余额相关路由
		balances := api.Group("/balances")
		{
			balances.GET("", balanceController.GetAllBalances)
			balances.GET("/:address", balanceController.GetUserBalance)
			balances.GET("/:address/history", balanceController.GetBalanceHistory)
			balances.POST("/:address/rebuild", balanceController.RebuildUserBalance)
		}

		// 积分相关路由
		points := api.Group("/points")
		{
			points.GET("", pointsController.GetPointsLeaderboard)
			points.GET("/stats", pointsController.GetPointsStats)
			points.POST("/calculate", pointsController.TriggerCalculation)
			points.GET("/:address", pointsController.GetUserPoints)
			points.GET("/:address/history", pointsController.GetPointsHistory)
		}

		// 链相关路由
		chains := api.Group("/chains")
		{
			chains.GET("", chainController.GetAllChains)
			chains.GET("/configs", chainController.GetChainConfigs)
			chains.GET("/points-rate", chainController.GetPointsRate)
			chains.POST("/points-rate", chainController.UpdatePointsRate)
			chains.GET("/:id", chainController.GetChainByID)
			chains.POST("/:id/sync", chainController.SyncChain)
		}
	}
}
