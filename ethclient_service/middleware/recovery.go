package middleware

import (
	"ethclient_service/logger"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Log.WithFields(logrus.Fields{
					"path":   c.Request.URL.Path,
					"method": c.Request.Method,
					"err":    err,
					"ip":     c.ClientIP(),
				}).Error("服务器内部异常")

				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "服务器内部异常",
				})
				// 中止请求处理
				c.Abort()
			}
		}()
		c.Next()
	}
}
