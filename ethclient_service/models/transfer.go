package models

import (
	"time"

	"gorm.io/gorm"
)

type Transfer struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	BlockNum  int64          `gorm:"not null;index" json:"block_num"`
	TxHash    string         `gorm:"size:66;not null;uniqueIndex:idx_txhash_logidx" json:"tx_hash"`
	LogIdx    int            `gorm:"not null;uniqueIndex:idx_txhash_logidx" json:"log_idx"`
	FromAddr  string         `gorm:"size:42;index" json:"from_addr"`
	ToAddr    string         `gorm:"size:42;index" json:"to_addr"`
	Amount    string         `gorm:"type:numeric(78);not null" json:"amount"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
