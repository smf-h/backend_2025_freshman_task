package auth

import (
	"net/http"
	"strings"

	"awesomeProject/internal/db"
	"awesomeProject/internal/models"

	"github.com/gin-gonic/gin"
)

// Context keys
const (
	CtxUserIDKey   = "user_id"
	CtxUsernameKey = "username"
	CtxUserLevel   = "user_level"
	CtxUserRole    = "user_role"
)

// AuthRequired JWT 鉴权中间件，失败统一返回 40101
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "msg": "未提供或非法的 Authorization 头"})
			return
		}
		tokenStr := strings.TrimSpace(authz[7:])
		claims, err := ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "msg": "token 无效或已过期"})
			return
		}
		// 读取用户（需要等级/角色）
		var user models.User
		if err := db.Global.First(&user, claims.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "msg": "用户不存在"})
			return
		}
		c.Set(CtxUserIDKey, user.ID)
		c.Set(CtxUsernameKey, user.Username)
		c.Set(CtxUserLevel, user.Level)
		c.Set(CtxUserRole, user.Role)
		c.Next()
	}
}

// HasAdminRole 判断是否管理员
func HasAdminRole(c *gin.Context) bool {
	v, ok := c.Get(CtxUserRole)
	if !ok {
		return false
	}
	role, _ := v.(string)
	return role == "admin"
}

// GetUserLevel 获取用户等级，失败返回 0
func GetUserLevel(c *gin.Context) int {
	v, ok := c.Get(CtxUserLevel)
	if !ok {
		return 0
	}
	if lv, ok2 := v.(int); ok2 {
		return lv
	}
	return 0
}
