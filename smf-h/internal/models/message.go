package models

import "time"

// Message 持久化一条会话消息
type Message struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID string    `gorm:"type:varchar(64);index;not null;constraint:OnDelete:CASCADE,OnUpdate:CASCADE;" json:"conversation_id"`
	UserID         uint      `gorm:"index;not null" json:"user_id"`
	Role           string    `gorm:"type:varchar(16);index;not null" json:"role"` // user / assistant / system
	Content        string    `gorm:"type:mediumtext;not null" json:"content"`
	TokenCount     int       `gorm:"type:int;default:0" json:"token_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Message) TableName() string { return "messages" }
