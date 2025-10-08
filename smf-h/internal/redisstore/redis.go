package redisstore

import (
	"context"
	"fmt"
	"time"

	"awesomeProject/internal/config"

	"github.com/redis/go-redis/v9"
)

// Client 全局 Redis 客户端 (可为空表示未启用)
var Client *redis.Client

// available 标记是否初始化成功
var available bool

// IsAvailable Redis 是否可用
func IsAvailable() bool { return available }

// Init 初始化 Redis 连接
func Init(cfg config.RedisConfig) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if cfg.PoolSize == 0 {
		cfg.PoolSize = 20
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 2 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 1 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 1 * time.Second
	}

	// 先创建临时 client，测试通过后再赋值全局
	tmp := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tmp.Ping(ctx).Err(); err != nil {
		available = false
		return err
	}
	Client = tmp
	available = true
	return nil
}

// Suggested cache key patterns
const (
	KeyUserTokenPrefix       = "chat:user:token:"        // userID -> last issued token (optional for invalidate)
	KeyConversationSummary   = "chat:conv:summary:"      // convID -> brief summary json
	KeyConversationLock      = "chat:conv:lock:"         // convID -> distributed lock
	KeyRateLimitPrefix       = "chat:ratelimit:"         // userID -> counts
	KeyModelUsageDailyPrefix = "chat:model:usage:daily:" // modelID:YYYYMMDD -> int
	KeyConversationRecent    = "chat:conv:recent:"       // convID -> recent messages json
)

// CacheRecentMessages 缓存最近会话消息(JSON)，ttl 可为 0 表示不设置过期
func CacheRecentMessages(ctx context.Context, conversationID string, json string, ttl time.Duration) {
	if !available || Client == nil {
		return
	}
	key := KeyConversationRecent + conversationID
	if ttl > 0 {
		_ = Client.Set(ctx, key, json, ttl).Err()
	} else {
		_ = Client.Set(ctx, key, json, 0).Err()
	}
}

// GetRecentMessages 读取最近会话缓存
func GetRecentMessages(ctx context.Context, conversationID string) (string, error) {
	if !available || Client == nil {
		return "", nil
	}
	key := KeyConversationRecent + conversationID
	val, err := Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// DeleteRecentMessages 删除会话的最近消息缓存
func DeleteRecentMessages(ctx context.Context, conversationID string) {
	if !available || Client == nil || conversationID == "" {
		return
	}
	key := KeyConversationRecent + conversationID
	_ = Client.Del(ctx, key).Err()
}

// DeleteConversationAll 删除一个会话相关的所有缓存键（目前 summary + recent）
func DeleteConversationAll(ctx context.Context, conversationID string) {
	if !available || Client == nil || conversationID == "" {
		return
	}
	keys := []string{KeyConversationRecent + conversationID, KeyConversationSummary + conversationID}
	_ = Client.Del(ctx, keys...).Err()
}
