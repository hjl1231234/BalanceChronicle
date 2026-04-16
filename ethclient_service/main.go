package main

import (
	"ethclient_service/config"
	"ethclient_service/database"
	"ethclient_service/logger"
	"ethclient_service/middleware"
	"ethclient_service/models"
	"ethclient_service/rabbitmq"
	"ethclient_service/routes"
	"ethclient_service/services"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig(".")
	if err != nil {
		logger.Log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志（只初始化一次）
	logger.InitLogger(&cfg)
	logger.Log.Info("配置加载成功")
	fmt.Println("配置加载成功")

	// 初始化数据库
	if err := database.InitDB(&cfg); err != nil {
		logger.Log.Fatalf("数据库初始化失败: %v", err)
	}
	defer database.CloseDB()

	// 自动迁移数据库
	if err := models.AutoMigrate(database.DB); err != nil {
		logger.Log.Fatalf("数据库迁移失败: %v", err)
	}
	logger.Log.Info("数据库迁移完成")

	// 创建积分计算服务
	pointsCalculator := services.NewPointsCalculator(&cfg)

	// 初始化RabbitMQ客户端
	var rabbitMQClient *rabbitmq.Client
	if cfg.RabbitMQ.Host != "" {
		rabbitMQClient, err = rabbitmq.NewClient(cfg.RabbitMQ)
		if err != nil {
			logger.Log.Errorf("初始化RabbitMQ失败: %v", err)
		} else {
			logger.Log.Info("✅ RabbitMQ初始化成功")
		}
	}

	// 创建事件监听服务
	eventListener := services.NewEventListener(&cfg)
	if rabbitMQClient != nil {
		eventListener.SetRabbitMQClient(rabbitMQClient)
	}

	// 启动事件监听服务
	if err := eventListener.Start(); err != nil {
		logger.Log.Errorf("启动事件监听服务失败: %v", err)
	}

	// 创建并启动事件消费者
	var eventConsumer *services.EventConsumer
	if rabbitMQClient != nil {
		eventConsumer = services.NewEventConsumer(&cfg)
		eventConsumer.SetRabbitMQClient(rabbitMQClient)
		if err := eventConsumer.Start(); err != nil {
			logger.Log.Errorf("启动事件消费者失败: %v", err)
		}
	}

	// 启动积分计算服务
	pointsCalculator.Start()

	// 创建 Gin 路由
	router := gin.New()
	router.Use(middleware.RecoveryMiddleware())

	// 设置路由
	routes.SetupRoutes(router, &cfg, pointsCalculator)

	// 设置优雅关闭
	setupGracefulShutdown(eventListener, pointsCalculator, eventConsumer, rabbitMQClient)

	// 启动服务器
	logger.Log.Infof("🚀 服务器启动，监听端口 %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		logger.Log.Fatalf("服务启动失败: %v", err)
	}
}

// setupGracefulShutdown 设置优雅关闭
func setupGracefulShutdown(eventListener *services.EventListener, pointsCalculator *services.PointsCalculator, eventConsumer *services.EventConsumer, rabbitMQClient *rabbitmq.Client) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Log.Info("\n👋 收到关闭信号，正在关闭服务...")

		// 停止事件消费者
		if eventConsumer != nil {
			eventConsumer.Stop()
		}

		// 停止事件监听服务
		eventListener.Stop()

		// 停止积分计算服务
		pointsCalculator.Stop()

		// 关闭RabbitMQ连接
		if rabbitMQClient != nil {
			rabbitMQClient.Close()
		}

		logger.Log.Info("✅ 服务已安全关闭")
		os.Exit(0)
	}()
}
