package models

import "time"

// Conversation 仅存储会话元数据和归属用户，消息仍在内存
type Conversation struct {
	ID           string    `gorm:"type:varchar(64);primaryKey" json:"id"`
	UserID       uint      `gorm:"index;not null" json:"user_id"`
	LastActiveAt time.Time `gorm:"index" json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Conversation) TableName() string { return "conversations" }
