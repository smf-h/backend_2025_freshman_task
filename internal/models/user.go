package models

import "time"

// User 基础用户实体
// 使用显式 varchar 类型，避免 MySQL 在 utf8mb4 下创建索引时报错 (BLOB/TEXT 需要前缀长度)
// password 存放 bcrypt hash (~60 字符) 预留 255
// 允许 Username 或 Email 其一为空，但至少应用层约束必须提供一个（注册时已校验）
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"type:varchar(64);uniqueIndex;comment:用户名" json:"username"`
	Email     string    `gorm:"type:varchar(128);uniqueIndex;comment:邮箱" json:"email"`
	Password  string    `gorm:"type:varchar(255);not null;comment:密码hash" json:"-"`
	Level     int       `gorm:"type:int;default:1;comment:用户等级(数值越大权限越高)" json:"level"`
	Role      string    `gorm:"type:varchar(32);default:user;comment:角色: user/admin" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }
