package ethereum

import (
	"ethclient_service/config"
	"ethclient_service/logger"
	"fmt"
	"sync"
)

// DualModeClientManager 双模式客户端管理器
type DualModeClientManager struct {
	clients map[string]*DualModeClient
	lock    sync.RWMutex
}

// NewDualModeClientManager 创建双模式客户端管理器
func NewDualModeClientManager() *DualModeClientManager {
	return &DualModeClientManager{
		clients: make(map[string]*DualModeClient),
	}
}

// AddClient 添加客户端
func (cm *DualModeClientManager) AddClient(name string, chainConfig config.ChainConfig, pollInterval int) error {
	client, err := NewDualModeClient(chainConfig, pollInterval)
	if err != nil {
		return err
	}

	cm.lock.Lock()
	cm.clients[name] = client
	cm.lock.Unlock()

	logger.Log.Infof("链 %s 双模式客户端初始化成功 (WS: %v)", name, chainConfig.UseWebSocket)
	return nil
}

// GetClient 获取客户端
func (cm *DualModeClientManager) GetClient(name string) (*DualModeClient, bool) {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	client, ok := cm.clients[name]
	return client, ok
}

// StartClient 启动指定客户端
func (cm *DualModeClientManager) StartClient(name string) error {
	cm.lock.RLock()
	client, ok := cm.clients[name]
	cm.lock.RUnlock()

	if !ok {
		logger.Log.Errorf("客户端 %s 不存在", name)
		return fmt.Errorf("客户端 %s 不存在", name)
	}

	return client.Start()
}

// StartAll 启动所有客户端
func (cm *DualModeClientManager) StartAll() {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	for name, client := range cm.clients {
		if err := client.Start(); err != nil {
			logger.Log.Errorf("启动客户端 %s 失败: %v", name, err)
		}
	}
}

// StopClient 停止指定客户端
func (cm *DualModeClientManager) StopClient(name string) {
	cm.lock.RLock()
	client, ok := cm.clients[name]
	cm.lock.RUnlock()

	if ok {
		client.Stop()
	}
}

// StopAll 停止所有客户端
func (cm *DualModeClientManager) StopAll() {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	for name, client := range cm.clients {
		client.Stop()
		logger.Log.Infof("链 %s 双模式客户端已停止", name)
	}
}

// CloseAll 关闭所有客户端 (兼容旧接口)
func (cm *DualModeClientManager) CloseAll() {
	cm.StopAll()
}

// GetAllClients 获取所有客户端
func (cm *DualModeClientManager) GetAllClients() map[string]*DualModeClient {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	// 返回副本
	result := make(map[string]*DualModeClient)
	for k, v := range cm.clients {
		result[k] = v
	}
	return result
}
