package models

import (
	"time"

	"gorm.io/gorm"
)

// Chain 链配置表
type Chain struct {
	ID                 string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ChainID            string    `gorm:"uniqueIndex;not null;size:20" json:"chain_id"` // 改为string，如 "1", "137", "31337"
	Name               string    `gorm:"size:100;not null" json:"name"`
	RPCURL             string    `gorm:"size:500" json:"rpc_url"`
	ContractAddress    string    `gorm:"size:42;not null" json:"contract_address"`
	BlockConfirmations int       `gorm:"default:6" json:"block_confirmations"`
	IsActive           bool      `gorm:"default:true" json:"is_active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// SyncState 事件同步状态表
type SyncState struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ChainID         string    `gorm:"uniqueIndex;size:36;not null" json:"chain_id"`
	LastSyncedBlock int64     `gorm:"not null" json:"last_synced_block"`
	IsSyncing       bool      `gorm:"default:false" json:"is_syncing"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Event 事件记录表
type Event struct {
	ID              string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ChainID         string     `gorm:"size:36;not null;index:idx_event_chain_block" json:"chain_id"`
	BlockNumber     int64      `gorm:"not null;index:idx_event_chain_block" json:"block_number"`
	BlockHash       string     `gorm:"size:66;not null" json:"block_hash"`
	TransactionHash string     `gorm:"size:66;not null;index:idx_event_tx_log" json:"transaction_hash"`
	LogIndex        int        `gorm:"not null;index:idx_event_tx_log" json:"log_index"`
	EventType       string     `gorm:"size:50;not null" json:"event_type"` // Transfer, Mint, Burn
	FromAddress     *string    `gorm:"size:42" json:"from_address"`
	ToAddress       *string    `gorm:"size:42" json:"to_address"`
	Amount          string     `gorm:"type:text;not null" json:"amount"`
	IsProcessed     bool       `gorm:"default:false;index" json:"is_processed"`
	ConfirmedAt     *time.Time `json:"confirmed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// BalanceChange 用户余额变动记录表
// 删除冗余字段：TokenAddress（通过Event->Chain获取）
type BalanceChange struct {
	ID            string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	EventID       string    `gorm:"size:36;not null" json:"event_id"`
	UserAddress   string    `gorm:"size:42;not null;index:idx_bc_user_time" json:"user_address"`
	ChainID       string    `gorm:"size:36;not null;index" json:"chain_id"`
	ChangeAmount  string    `gorm:"type:text;not null" json:"change_amount"` // 正数为增加，负数为减少
	BalanceBefore string    `gorm:"type:text;not null" json:"balance_before"`
	BalanceAfter  string    `gorm:"type:text;not null" json:"balance_after"`
	ChangeType    string    `gorm:"size:50;not null" json:"change_type"` // mint, burn, transfer_in, transfer_out
	BlockNumber   int64     `gorm:"not null" json:"block_number"`
	Timestamp     time.Time `gorm:"not null;index:idx_bc_user_time" json:"timestamp"`
	CreatedAt     time.Time `json:"created_at"`
}

// UserBalance 用户总余额表
// 删除冗余字段：TokenAddress（每个链只有一个合约）
type UserBalance struct {
	ID            string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserAddress   string    `gorm:"size:42;not null" json:"user_address"`
	ChainID       string    `gorm:"size:36;not null" json:"chain_id"`
	Balance       string    `gorm:"type:text;not null;index" json:"balance"` // 用于排行榜排序
	LastUpdatedAt time.Time `json:"last_updated_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PointsCalculation 积分计算记录表
type PointsCalculation struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserAddress     string    `gorm:"size:42;not null;index:idx_pc_user_time" json:"user_address"`
	ChainID         string    `gorm:"size:36;not null;index" json:"chain_id"`
	StartTime       time.Time `gorm:"not null;index:idx_pc_user_time" json:"start_time"`
	EndTime         time.Time `gorm:"not null;index:idx_pc_user_end" json:"end_time"`
	Balance         string    `gorm:"type:text;not null" json:"balance"`
	DurationMinutes float64   `gorm:"not null" json:"duration_minutes"`
	PointsEarned    string    `gorm:"type:text;not null" json:"points_earned"`
	CalculationTime time.Time `json:"calculation_time"`
}

// UserPoints 用户总积分表
type UserPoints struct {
	ID               string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserAddress      string    `gorm:"size:42;not null" json:"user_address"`
	ChainID          *string   `gorm:"size:36" json:"chain_id"` // nil表示全链统计
	TotalPoints      string    `gorm:"type:text;not null" json:"total_points"`
	LastCalculatedAt time.Time `json:"last_calculated_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName 指定表名
func (UserBalance) TableName() string {
	return "user_balances"
}

func (BalanceChange) TableName() string {
	return "balance_changes"
}

func (PointsCalculation) TableName() string {
	return "points_calculations"
}

func (UserPoints) TableName() string {
	return "user_points"
}

// AutoMigrate 自动迁移所有模型
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Chain{},
		&SyncState{},
		&Event{},
		&BalanceChange{},
		&UserBalance{},
		&PointsCalculation{},
		&UserPoints{},
	)
}

// DropAllTables 删除所有表（用于重新初始化）
func DropAllTables(db *gorm.DB) error {
	// 按依赖关系倒序删除
	tables := []string{
		"user_points",
		"points_calculations",
		"user_balances",
		"balance_changes",
		"events",
		"sync_states",
		"chains",
	}

	for _, table := range tables {
		if err := db.Migrator().DropTable(table); err != nil {
			return err
		}
	}
	return nil
}
