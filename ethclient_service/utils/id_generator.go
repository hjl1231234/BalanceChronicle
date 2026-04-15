package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateID 生成唯一ID
func GenerateID() string {
	// 使用时间和随机数生成唯一ID
	timestamp := time.Now().UnixNano()
	
	// 生成8字节随机数
	b := make([]byte, 8)
	rand.Read(b)
	
	return fmt.Sprintf("%d%s", timestamp, hex.EncodeToString(b))
}

// GenerateChainID 生成链ID
func GenerateChainID(chainID string) string {
	return fmt.Sprintf("chain_%s", chainID)
}
