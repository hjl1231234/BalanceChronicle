package main

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/ethereum"
	"ethclient_service/logger"
	"ethclient_service/middleware"
	"ethclient_service/models"
	"ethclient_service/routes"
	"fmt"

	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化日志
	logger.InitLogger(nil)
	// 加载配置
	cfg, err := config.LoadConfig("../")
	if err != nil {
		// logger.Log.Fatalf("加载配置失败: %v", err)
		// 这样会有nil指针问题
		logger.Log.Fatalf("加载配置失败: %v", err)
	}
	logger.Log.Info("配置加载成功")
	fmt.Println("配置加载成功")

	logger.InitLogger(&cfg)

	database.InitDB(&cfg)
	if err := database.DB.AutoMigrate(
		&models.Transfer{},
		&models.Balance{},
	); err != nil {
		logger.Log.Fatalf("数据库迁移失败: %v", err)
	}
	logger.Log.Info("数据库迁移完成")

	// 初始化并启动Token索引服务
	indexer, err := ethereum.NewTokenIndexer(&cfg)
	if err != nil {
		logger.Log.Fatalf("初始化Token索引服务失败: %v", err)
	}
	indexer.Start()

	router := gin.New()

	router.Use(middleware.RecoveryMiddleware())

	// 全局JWT中间件，也可以在路由组中单独设置
	// router.Use(middleware.JWTUserMiddleware(&cfg))

	routes.SetupRoutes(router, &cfg)

	logger.Log.Infof("服务器启动，监听端口 %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		logger.Log.Fatalf("服务启动失败: %v", err)
	}

}
