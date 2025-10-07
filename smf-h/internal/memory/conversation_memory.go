package memory

import (
	"errors"
	"sync"
)

// Message 表示一条对话消息
// Role 典型值: user / assistant / system
// Content 为文本内容
// TokenCount 可选，用于后续做上下文裁剪策略
// 这里简化不做实际 token 计算，可在外部调用模型前估算

type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	TokenCount int    `json:"token_count,omitempty"`
}

// ConversationMemory 用于管理单个会话的消息列表
// 内部线程安全，可被多个 goroutine 并发使用

type ConversationMemory struct {
	mu      sync.RWMutex
	maxMsgs int // 限制最多保留的消息条数 (简单策略)
	msgs    []Message
}

// NewConversationMemory 创建一个新的会话记忆对象
// maxMsgs: 为 0 或负数时表示不限制条数（但生产环境建议限制以防内存膨胀）
func NewConversationMemory(maxMsgs int) *ConversationMemory {
	return &ConversationMemory{maxMsgs: maxMsgs, msgs: make([]Message, 0)}
}

// Append 追加一条消息
// 如果超出 maxMsgs，会丢弃最早的消息（先进先出）
func (c *ConversationMemory) Append(m Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	if c.maxMsgs > 0 && len(c.msgs) > c.maxMsgs {
		// 丢弃最前面多出的部分，只保留最后 maxMsgs 条
		over := len(c.msgs) - c.maxMsgs
		c.msgs = c.msgs[over:]
	}
}

// GetAll 返回当前所有消息的副本，防止外部修改内部切片
func (c *ConversationMemory) GetAll() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]Message, len(c.msgs))
	copy(res, c.msgs)
	return res
}

// LastN 返回最近的 N 条消息（不足 N 则返回全部）
func (c *ConversationMemory) LastN(n int) []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n <= 0 || n >= len(c.msgs) {
		res := make([]Message, len(c.msgs))
		copy(res, c.msgs)
		return res
	}
	res := make([]Message, n)
	copy(res, c.msgs[len(c.msgs)-n:])
	return res
}

// TruncateByTokens 按“估算的 token 数”从头开始裁剪，直到总 token 不超过 limit
// 如果单条消息 token 已超过 limit 则返回错误
// 这里假设 Message.TokenCount 已在外部填充
func (c *ConversationMemory) TruncateByTokens(limit int) error {
	if limit <= 0 {
		return errors.New("limit must be positive")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var total int
	for _, m := range c.msgs {
		if m.TokenCount > limit {
			return errors.New("single message exceeds token limit")
		}
		total += m.TokenCount
	}
	// 若总量本就不超，直接返回
	if total <= limit {
		return nil
	}
	// 从最前面开始移除，直到满足条件
	idx := 0
	for idx < len(c.msgs) && total > limit {
		removed := c.msgs[idx].TokenCount
		total -= removed
		idx++
	}
	c.msgs = c.msgs[idx:]
	return nil
}

// Clear 清空会话
func (c *ConversationMemory) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = c.msgs[:0]
}

// ------- 多会话管理器 -------

// MemoryManager 管理多个会话ID -> ConversationMemory
// 适合用在内存版快速原型。生产可换成 Redis / 数据库 + 缓存。
type MemoryManager struct {
	mu       sync.RWMutex
	sessions map[string]*ConversationMemory
	defaultN int // 每个会话默认最多消息条数限制
}

// NewMemoryManager 创建管理器
func NewMemoryManager(defaultMaxMsgs int) *MemoryManager {
	return &MemoryManager{
		sessions: make(map[string]*ConversationMemory),
		defaultN: defaultMaxMsgs,
	}
}

// Get 会返回指定会话ID的记忆对象，不存在则新建
func (m *MemoryManager) Get(conversationID string) *ConversationMemory {
	m.mu.RLock()
	cm, ok := m.sessions[conversationID]
	m.mu.RUnlock()
	if ok {
		return cm
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// 双检查
	if cm, ok = m.sessions[conversationID]; ok {
		return cm
	}
	cm = NewConversationMemory(m.defaultN)
	m.sessions[conversationID] = cm
	return cm
}

// Delete 删除一个会话
func (m *MemoryManager) Delete(conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, conversationID)
}

// ListIDs 列出当前所有会话ID （用于调试）
func (m *MemoryManager) ListIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}
