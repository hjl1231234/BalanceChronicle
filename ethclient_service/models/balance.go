package models

import (
	"time"

	"gorm.io/gorm"
)

type Balance struct {
	Address           string         `gorm:"primaryKey;size:42" json:"address"`
	Balance           string         `gorm:"type:numeric(78);not null" json:"balance"`
	LastUpdatedBlock  int64          `gorm:"not null;index" json:"last_updated_block"`
	UpdatedAt         time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	CreatedAt         time.Time      `gorm:"autoCreateTime" json:"created_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}
