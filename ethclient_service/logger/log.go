package logger

import (
	"ethclient_service/config"
	"os"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

func InitLogger(cfg *config.Config) {

	Log = logrus.New()

	level := logrus.InfoLevel
	if cfg != nil && cfg.LogLevel != "" {

		parsedLevel, err := logrus.ParseLevel(cfg.LogLevel)
		if err != nil {
			Log.Warnf("无效的日志级别 '%s'，使用默认级别 'info'", cfg.LogLevel)
		} else {
			level = parsedLevel
		}
	}
	Log.SetLevel(level)

	// 添加默认格式化器
	Log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 设置标准输出
	Log.SetOutput(os.Stdout)

	Log.Info("日志系统初始化完成")
}
