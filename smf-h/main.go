package main

// @title AI Chat API
// @version 0.4
// @description 简化版 AI 问答系统接口，包含认证/聊天/会话与模型权限示例。
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"

	_ "awesomeProject/docs/swagger" // swagger 文档注册 (确保 /swagger/doc.json 可用)
	"awesomeProject/internal/auth"
	"awesomeProject/internal/config"
	"awesomeProject/internal/db"
	"awesomeProject/internal/memory"
	"awesomeProject/internal/models"
	"awesomeProject/internal/redisstore"
)

// 全局状态
var (
	mgr             *memory.MemoryManager
	defaultModelID  string
	maxHistoryToUse int
	client          *arkruntime.Client
	appConfig       config.Root
	modelLevelMap   map[string]int // modelID -> min_level
)

// ChatRequest 入参
type ChatRequest struct {
	ConversationID string `json:"conversation_id" example:""`
	Question       string `json:"question" example:"你好"`
	Model          string `json:"model" example:"doubao-seed-1-6-250615"`
}

// ChatResponse 出参
type ChatResponse struct {
	ConversationID string           `json:"conversation_id"`
	Answer         string           `json:"answer"`
	UsedHistory    int              `json:"used_history"`
	Messages       []memory.Message `json:"messages,omitempty"`
}

// APIResponse 通用响应包装
type APIResponse struct {
	Code int         `json:"code" example:"0"`
	Msg  string      `json:"msg" example:"ok"`
	Data interface{} `json:"data,omitempty"`
}

// ---- 更具体的响应包装（用于 swagger 展示结构） ----
type AuthData struct {
	Token  string `json:"token" example:"eyJhbGciOiJIUzI1NiIs..."`
	UserID uint   `json:"user_id" example:"1"`
}
type AuthResponse struct {
	Code int      `json:"code" example:"0"`
	Msg  string   `json:"msg" example:"ok"`
	Data AuthData `json:"data"`
}

type ChatSendData struct {
	ConversationID string `json:"conversation_id" example:"c3a2f1c4-uuid"`
	Answer         string `json:"answer" example:"你好，我是AI助手"`
	UsedHistory    int    `json:"used_history" example:"2"`
}
type ChatSendResponse struct {
	Code int          `json:"code" example:"0"`
	Msg  string       `json:"msg" example:"ok"`
	Data ChatSendData `json:"data"`
}

type ChatHistoryData struct {
	ConversationID string           `json:"conversation_id"`
	Messages       []memory.Message `json:"messages"`
	Count          int              `json:"count" example:"4"`
}
type ChatHistoryResponse struct {
	Code int             `json:"code" example:"0"`
	Msg  string          `json:"msg" example:"ok"`
	Data ChatHistoryData `json:"data"`
}

type ChatClearData struct {
	ConversationID string `json:"conversation_id"`
	Deleted        bool   `json:"deleted"`
	Full           bool   `json:"full"`
	MessageDeleted int64  `json:"message_deleted" example:"4"` // 实际删除的消息条数
}
type ChatClearResponse struct {
	Code int           `json:"code" example:"0"`
	Msg  string        `json:"msg" example:"ok"`
	Data ChatClearData `json:"data"`
}

// ---------------- 错误结构 与 常量 ----------------
type APIError struct {
	Code int    `json:"code" example:"40001"`
	Msg  string `json:"msg" example:"参数错误"`
}

type BadRequestError struct {
	Code int    `json:"code" example:"40001"`
	Msg  string `json:"msg" example:"参数错误: question 必填"`
}
type ModelNotFoundError struct {
	Code int    `json:"code" example:"40002"`
	Msg  string `json:"msg" example:"模型不存在或未配置"`
}
type UnauthorizedError struct {
	Code int    `json:"code" example:"40101"`
	Msg  string `json:"msg" example:"未提供或非法的 Authorization 头"`
}
type ForbiddenModelError struct {
	Code int    `json:"code" example:"40301"`
	Msg  string `json:"msg" example:"无权限使用模型(doubao-seed-1-6-250615)，需要等级≥3"`
}
type ForbiddenConversationError struct {
	Code int    `json:"code" example:"40302"`
	Msg  string `json:"msg" example:"会话不属于当前用户"`
}
type InternalModelCallError struct {
	Code int    `json:"code" example:"50000"`
	Msg  string `json:"msg" example:"模型调用失败"`
}
type InternalEmptyReturnError struct {
	Code int    `json:"code" example:"50001"`
	Msg  string `json:"msg" example:"模型返回为空"`
}

const (
	CodeOK                    = 0
	CodeBadParam              = 40001
	CodeModelNotFound         = 40002
	CodeUnauthorized          = 40101
	CodeForbiddenModel        = 40301
	CodeForbiddenConversation = 40302
	CodeInternalError         = 50000
	CodeModelEmpty            = 50001
)

func writeErr(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, APIError{Code: code, Msg: msg})
}

func main() {
	// 1. 加载配置（允许没有文件时使用硬编码默认配置，方便本地快速运行不提交真实配置）
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		fmt.Println("[WARN] 未找到或解析配置文件，将使用内置开发默认配置:", err)
		cfg = config.Root{
			Server: config.ServerConfig{Port: 8080, Mode: "debug"},
			MySQL:  config.MySQLConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: "", Database: "ai_chat", Charset: "utf8mb4"},
			Chat:   config.ChatConfig{DefaultModel: "doubao-seed-1-6-250615", MaxHistory: 12, MemoryLimitMsgs: 200, RequestTimeout: 15 * time.Second},
			Auth:   config.AuthConfig{JWTSecret: "dev_local_jwt_secret", AccessTTL: 24 * time.Hour},
			Models: []config.ModelPolicy{{ID: "doubao-seed-1-6-250615", MinLevel: 1}, {ID: "deepseek-v3-1-terminus", MinLevel: 3}},
			Redis:  config.RedisConfig{Host: "127.0.0.1", Port: 6379, DB: 0, PoolSize: 20, DialTimeout: 2 * time.Second, ReadTimeout: 1 * time.Second, WriteTimeout: 1 * time.Second},
		}
		// 仍允许环境变量覆盖
		config.ApplyEnvOverrides(&cfg)
	}
	appConfig = cfg

	// 2. 初始化内存与参数
	defaultModelID = cfg.Chat.DefaultModel
	maxHistoryToUse = cfg.Chat.MaxHistory
	mgr = memory.NewMemoryManager(cfg.Chat.MemoryLimitMsgs)

	// 构建模型权限 map
	modelLevelMap = make(map[string]int)
	for _, mp := range cfg.Models {
		if mp.ID != "" {
			modelLevelMap[mp.ID] = mp.MinLevel
		}
	}
	// 确保默认模型至少进入策略，未配置则给予 min_level=1
	if _, ok := modelLevelMap[defaultModelID]; !ok && defaultModelID != "" {
		modelLevelMap[defaultModelID] = 1
	}

	// 3. AI 客户端（API KEY 依旧使用环境变量）
	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		fmt.Println("[WARN] 未设置 ARK_API_KEY，将使用 mock 模型回答 (本地开发模式)。设置 ARK_API_KEY 可启用真实模型调用。")
	} else {
		client = arkruntime.NewClientWithApiKey(apiKey, arkruntime.WithBaseUrl("https://ark.cn-beijing.volces.com/api/v3"))
	}

	// 3.1 设置 JWT 密钥
	auth.SetJWTSecret(cfg.Auth.JWTSecret)

	// 4. 初始化 MySQL
	if err := db.Init(db.Config{
		User:     cfg.MySQL.User,
		Password: cfg.MySQL.Password,
		Host:     cfg.MySQL.Host,
		Port:     cfg.MySQL.Port,
		Database: cfg.MySQL.Database,
		Charset:  cfg.MySQL.Charset,
	}); err != nil {
		panic("MySQL 连接失败: " + err.Error())
	}

	// 4.1 初始化 Redis（可选，失败记录日志并降级为未启用）
	if err := redisstore.Init(cfg.Redis); err != nil {
		fmt.Println("[WARN] Redis 未启用，原因:", err.Error())
	}

	// 5. Gin 模式
	if cfg.Server.Mode != "" {
		gin.SetMode(cfg.Server.Mode)
	}
	r := gin.Default()

	// 4.2 AutoMigrate 用户表 (放在路由前)
	if err := db.Global.AutoMigrate(&models.User{}, &models.Conversation{}, &models.Message{}); err != nil {
		panic("自动迁移失败: " + err.Error())
	}

	// Swagger 文档路由（可选：生产环境可关闭）
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 路由
	// 健康检查
	// @Summary 健康检查
	// @Tags System
	// @Produce json
	// @Success 200 {object} map[string]string
	// @Router /health [get]
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	// Auth 路由
	r.POST("/api/auth/register", handleRegister)
	r.POST("/api/auth/login", handleLogin)
	// 需要鉴权的聊天路由
	chatGroup := r.Group("/api/chat", auth.AuthRequired())
	{
		chatGroup.POST("/send", handleChatSend)
		chatGroup.GET("/history", handleChatHistory)
		chatGroup.POST("/clear", handleChatClear)
		chatGroup.GET("/list", handleChatList)
		// 调试缓存接口（需在环境变量 DEBUG_CACHE=1 下调用）
		chatGroup.GET("/debug/cache", handleChatDebugCache)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	r.Run(addr)
}

// 发送聊天请求
// handleChatSend 发送聊天
// @Summary 发送消息
// @Description 发送用户问题并返回模型回答，支持新建或继续会话
// @Tags Chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ChatRequest true "请求体"
// @Success 200 {object} ChatSendResponse "成功"
// @Failure 400 {object} BadRequestError
// @Failure 400 {object} ModelNotFoundError
// @Failure 401 {object} UnauthorizedError
// @Failure 403 {object} ForbiddenModelError
// @Failure 403 {object} ForbiddenConversationError
// @Failure 500 {object} InternalModelCallError
// @Failure 500 {object} InternalEmptyReturnError
// @Router /api/chat/send [post]
func handleChatSend(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Question) == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "参数错误: question 必填")
		return
	}
	if req.ConversationID == "" {
		req.ConversationID = uuid.NewString()
		// 创建会话记录并归属当前用户
		uidVal, _ := c.Get(auth.CtxUserIDKey)
		uid, _ := uidVal.(uint)
		conv := models.Conversation{ID: req.ConversationID, UserID: uid, LastActiveAt: time.Now()}
		_ = db.Global.Create(&conv).Error
	}
	if req.Model == "" {
		req.Model = defaultModelID
	}

	// 会话归属验证（如果会话存在则检查 user_id）
	if req.ConversationID != "" {
		uidVal, _ := c.Get(auth.CtxUserIDKey)
		uid, _ := uidVal.(uint)
		var conv models.Conversation
		if err := db.Global.First(&conv, "id = ?", req.ConversationID).Error; err == nil {
			if conv.UserID != uid {
				writeErr(c, http.StatusForbidden, CodeForbiddenConversation, "会话不属于当前用户")
				return
			}
			// 更新活跃时间
			_ = db.Global.Model(&conv).Update("last_active_at", time.Now()).Error
		}
	}

	// 内存预热：如果该会话内存为空，尝试先用 Redis 最近窗口填充，失败再回源 DB
	warmConversationMemory(c.Request.Context(), req.ConversationID)

	// 模型存在性校验：未在策略表里直接判定为不存在
	if _, ok := modelLevelMap[req.Model]; !ok {
		writeErr(c, http.StatusBadRequest, CodeModelNotFound, "模型不存在或未配置")
		return
	}

	// 权限校验：获取用户等级（中间件已注入）
	userLevel := auth.GetUserLevel(c)
	minLv := modelLevelMap[req.Model]
	if userLevel < minLv && !auth.HasAdminRole(c) {
		writeErr(c, http.StatusForbidden, CodeForbiddenModel, fmt.Sprintf("无权限使用模型(%s)，需要等级≥%d", req.Model, minLv))
		return
	}

	cm := mgr.Get(req.ConversationID)
	userTokenCount := roughTokenEstimate(req.Question)
	cm.Append(memory.Message{Role: "user", Content: req.Question, TokenCount: userTokenCount})
	// 持久化用户消息
	uidVal, _ := c.Get(auth.CtxUserIDKey)
	uid, _ := uidVal.(uint)
	_ = db.Global.Create(&models.Message{ConversationID: req.ConversationID, UserID: uid, Role: "user", Content: req.Question, TokenCount: userTokenCount}).Error

	// 准备历史：之前只取 maxHistoryToUse，容易导致上下文太短；
	// 为了让模型看到更多往返，这里扩大到 *2（与缓存策略一致）。
	contextWindow := maxHistoryToUse * 2
	if contextWindow <= 0 {
		contextWindow = maxHistoryToUse
	}
	history := cm.LastN(contextWindow)
	modelMsgs := convertToModelMessages(history)

	var answer string
	if client == nil { // mock 模式
		answer = fmt.Sprintf("[mock:%s] 你说: %s", req.Model, req.Question)
	} else {
		resp, err := client.CreateChatCompletion(c.Request.Context(), model.CreateChatCompletionRequest{
			Model:    req.Model,
			Messages: modelMsgs,
		})
		if err != nil {
			writeErr(c, http.StatusInternalServerError, CodeInternalError, "模型调用失败")
			return
		}
		// 根据 SDK 结构：如果 Message 不是指针则无需判空；只校验 Choices 与 Content 指针
		if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == nil || resp.Choices[0].Message.Content.StringValue == nil {
			writeErr(c, http.StatusInternalServerError, CodeModelEmpty, "模型返回为空")
			return
		}
		answer = *resp.Choices[0].Message.Content.StringValue
	}
	ansToken := roughTokenEstimate(answer)
	cm.Append(memory.Message{Role: "assistant", Content: answer, TokenCount: ansToken})
	// 持久化助手回复
	_ = db.Global.Create(&models.Message{ConversationID: req.ConversationID, UserID: uid, Role: "assistant", Content: answer, TokenCount: ansToken}).Error

	// 写入最近历史缓存（用户+助手最新若干条）: 以数据库最终消息为准，从内存截取最近 maxHistoryToUse*2 作为展示缓存 (冗余一些)
	{
		recent := cm.LastN(maxHistoryToUse * 2)
		if b, err := json.Marshal(recent); err == nil {
			redisstore.CacheRecentMessages(c.Request.Context(), req.ConversationID, string(b), 10*time.Minute)
			if os.Getenv("DEBUG_CACHE") == "1" {
				fmt.Printf("[DEBUG_CACHE] set recent cache conv=%s size=%d\n", req.ConversationID, len(recent))
			}
		}
	}

	c.JSON(http.StatusOK, ChatSendResponse{Code: 0, Msg: "ok", Data: ChatSendData{
		ConversationID: req.ConversationID,
		Answer:         answer,
		UsedHistory:    len(history),
	}})
}

// warmConversationMemory 如果指定会话在内存中为空，则尝试：Redis -> DB 回填最近历史，避免服务重启后第一轮 /send 丢上下文。
func warmConversationMemory(ctx context.Context, conversationID string) {
	if conversationID == "" {
		return
	}
	cm := mgr.Get(conversationID)
	if len(cm.GetAll()) > 0 { // 已有内存
		return
	}
	// 1. Redis 最近历史
	if cached, err := redisstore.GetRecentMessages(ctx, conversationID); err == nil && cached != "" {
		var recent []memory.Message
		if jsonErr := json.Unmarshal([]byte(cached), &recent); jsonErr == nil {
			for _, m := range recent {
				cm.Append(m)
			}
			return
		}
	}
	// 2. 数据库全量（或可限制最大读取条数：这里简单读取全部，然后内存自身会保留最近 maxMsgs 条）
	var dbMsgs []models.Message
	if err := db.Global.Where("conversation_id = ?", conversationID).Order("id asc").Find(&dbMsgs).Error; err == nil {
		for _, m := range dbMsgs {
			cm.Append(memory.Message{Role: m.Role, Content: m.Content, TokenCount: m.TokenCount})
		}
	}
}

// 获取历史
// handleChatHistory 获取会话历史
// @Summary 获取历史
// @Tags Chat
// @Produce json
// @Security BearerAuth
// @Param conversation_id query string true "会话ID"
// @Success 200 {object} ChatHistoryResponse
// @Failure 400 {object} BadRequestError
// @Failure 401 {object} UnauthorizedError
// @Failure 403 {object} ForbiddenConversationError
// @Failure 500 {object} InternalModelCallError
// @Router /api/chat/history [get]
func handleChatHistory(c *gin.Context) {
	conversationID := c.Query("conversation_id")
	if conversationID == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "conversation_id 必填")
		return
	}
	// 所属校验
	uidVal, _ := c.Get(auth.CtxUserIDKey)
	uid, _ := uidVal.(uint)
	var conv models.Conversation
	if err := db.Global.First(&conv, "id = ?", conversationID).Error; err == nil {
		if conv.UserID != uid {
			writeErr(c, http.StatusForbidden, CodeForbiddenConversation, "会话不属于当前用户")
			return
		}
	}
	// 尝试从缓存读取最近历史（不一定是全量，用于快速返回）
	var dbMsgs []models.Message
	var fromCache bool
	if cached, err := redisstore.GetRecentMessages(c.Request.Context(), conversationID); err == nil && cached != "" {
		var recent []memory.Message
		if jsonErr := json.Unmarshal([]byte(cached), &recent); jsonErr == nil {
			// 将 recent 转换为 dbMsgs 兼容后续逻辑
			for _, m := range recent {
				dbMsgs = append(dbMsgs, models.Message{ConversationID: conversationID, Role: m.Role, Content: m.Content, TokenCount: m.TokenCount})
			}
			fromCache = true
		}
	}
	if !fromCache { // 缓存未命中 -> 读数据库全量
		if err := db.Global.Where("conversation_id = ?", conversationID).Order("id asc").Find(&dbMsgs).Error; err != nil {
			writeErr(c, http.StatusInternalServerError, CodeInternalError, "历史读取失败")
			return
		}
		// 回填缓存（截取最近 maxHistoryToUse*2）
		if len(dbMsgs) > 0 {
			limit := maxHistoryToUse * 2
			start := 0
			if len(dbMsgs) > limit {
				start = len(dbMsgs) - limit
			}
			recent := make([]memory.Message, 0, len(dbMsgs[start:]))
			for _, m := range dbMsgs[start:] {
				recent = append(recent, memory.Message{Role: m.Role, Content: m.Content, TokenCount: m.TokenCount})
			}
			if b, err := json.Marshal(recent); err == nil {
				redisstore.CacheRecentMessages(c.Request.Context(), conversationID, string(b), 10*time.Minute)
			}
		}
	}
	// 同步到内存（若需要后续继续对话）
	cm := mgr.Get(conversationID)
	if len(cm.GetAll()) == 0 { // 仅在内存为空时灌入，避免重复
		for _, m := range dbMsgs {
			cm.Append(memory.Message{Role: m.Role, Content: m.Content, TokenCount: m.TokenCount})
		}
	}
	// 组装返回结构
	returnMsgs := make([]memory.Message, 0, len(dbMsgs))
	for _, m := range dbMsgs {
		returnMsgs = append(returnMsgs, memory.Message{Role: m.Role, Content: m.Content, TokenCount: m.TokenCount})
	}
	c.JSON(http.StatusOK, ChatHistoryResponse{Code: 0, Msg: "ok", Data: ChatHistoryData{ConversationID: conversationID, Messages: returnMsgs, Count: len(returnMsgs)}})
}

// 清空会话
// handleChatClear 清空会话
// @Summary 清空会话
// @Tags Chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body map[string]string true "会话ID"
// @Success 200 {object} ChatClearResponse
// @Failure 400 {object} BadRequestError
// @Failure 401 {object} UnauthorizedError
// @Failure 403 {object} ForbiddenConversationError
// @Failure 500 {object} InternalModelCallError
// @Router /api/chat/clear [post]
func handleChatClear(c *gin.Context) {
	type clearReq struct {
		ConversationID string `json:"conversation_id"`
		Full           bool   `json:"full"` // 为 true 时连同会话记录一起删除
	}
	var r clearReq
	if err := c.ShouldBindJSON(&r); err != nil || r.ConversationID == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "conversation_id 必填")
		return
	}
	uidVal, _ := c.Get(auth.CtxUserIDKey)
	uid, _ := uidVal.(uint)
	var conv models.Conversation
	if err := db.Global.First(&conv, "id = ?", r.ConversationID).Error; err == nil {
		if conv.UserID != uid {
			writeErr(c, http.StatusForbidden, CodeForbiddenConversation, "会话不属于当前用户")
			return
		}
	}
	// 删除数据库消息并统计条数
	var delResult = db.Global.Where("conversation_id = ?", r.ConversationID).Delete(&models.Message{})
	if delResult.Error != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "清空失败")
		return
	}
	deletedConv := false
	if r.Full { // 连同会话元数据删除
		if err := db.Global.Delete(&models.Conversation{}, "id = ?", r.ConversationID).Error; err == nil {
			deletedConv = true
		}
	}
	// 清内存
	mgr.Get(r.ConversationID).Clear()
	if r.Full {
		mgr.Delete(r.ConversationID)
	}
	// 删除 Redis 缓存（recent + summary）
	redisstore.DeleteConversationAll(c.Request.Context(), r.ConversationID)
	if os.Getenv("DEBUG_CACHE") == "1" {
		fmt.Printf("[DEBUG_CACHE] delete cache conv=%s full=%v msg_deleted=%d redis_available=%v\n", r.ConversationID, r.Full, delResult.RowsAffected, redisstore.IsAvailable())
	}
	c.JSON(http.StatusOK, ChatClearResponse{Code: 0, Msg: "ok", Data: ChatClearData{ConversationID: r.ConversationID, Deleted: deletedConv, Full: r.Full, MessageDeleted: delResult.RowsAffected}})
}

// 调试查看缓存（只在 DEBUG_CACHE=1 时启用）
func handleChatDebugCache(c *gin.Context) {
	if os.Getenv("DEBUG_CACHE") != "1" {
		c.JSON(http.StatusForbidden, APIError{Code: CodeForbiddenModel, Msg: "not enabled"})
		return
	}
	convID := c.Query("conversation_id")
	if convID == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "conversation_id 必填")
		return
	}
	cached, _ := redisstore.GetRecentMessages(c.Request.Context(), convID)
	c.JSON(http.StatusOK, APIResponse{Code: 0, Msg: "ok", Data: gin.H{"conversation_id": convID, "cached_recent": cached, "length": len(cached)}})
}

// handleChatList 返回当前用户的会话 ID 列表
// @Summary 会话列表
// @Tags Chat
// @Produce json
// @Security BearerAuth
// @Success 200 {object} APIResponse
// @Failure 401 {object} UnauthorizedError
// @Router /api/chat/list [get]
func handleChatList(c *gin.Context) {
	uidVal, _ := c.Get(auth.CtxUserIDKey)
	uid, _ := uidVal.(uint)
	type convRow struct {
		ID           string    `json:"id"`
		LastActiveAt time.Time `json:"last_active_at"`
	}
	var rows []convRow
	if err := db.Global.Model(&models.Conversation{}).
		Select("id, last_active_at").
		Where("user_id = ?", uid).
		Order("last_active_at desc").
		Find(&rows).Error; err != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "查询失败")
		return
	}
	c.JSON(http.StatusOK, APIResponse{Code: 0, Msg: "ok", Data: gin.H{"conversations": rows, "count": len(rows)}})
}

// 工具: 将内存消息转为模型消息
func convertToModelMessages(msgs []memory.Message) []*model.ChatCompletionMessage {
	res := make([]*model.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		role := model.ChatMessageRoleUser
		if m.Role == "assistant" {
			role = model.ChatMessageRoleAssistant
		} else if m.Role == "system" {
			role = model.ChatMessageRoleSystem
		}
		res = append(res, &model.ChatCompletionMessage{Role: role, Content: &model.ChatCompletionMessageContent{StringValue: volcengine.String(m.Content)}})
	}
	return res
}

// 粗略 token 估算
func roughTokenEstimate(s string) int {
	if s == "" {
		return 0
	}
	return int(float64(len(strings.Fields(s))) * 1.3)
}

// -------------------- 用户注册与登录 --------------------

type registerReq struct {
	Username string `json:"username" binding:"required" example:"user1"`
	Email    string `json:"email" example:"user1@test.com"`
	Password string `json:"password" binding:"required" example:"Abcd1234!"`
}

type loginReq struct {
	Username string `json:"username" example:"user1"`
	Email    string `json:"email" example:"user1@test.com"`
	Password string `json:"password" binding:"required" example:"Abcd1234!"`
}

// handleRegister 简单注册：用户名唯一，密码 bcrypt
// handleRegister 用户注册
// @Summary 用户注册
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body registerReq true "注册参数"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} BadRequestError
// @Failure 500 {object} InternalModelCallError
// @Router /api/auth/register [post]
func handleRegister(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "参数错误")
		return
	}
	if req.Username == "" && req.Email == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "username 或 email 必填其一")
		return
	}
	// 检查是否已存在
	var existing models.User
	if err := db.Global.Where("username = ? OR email = ?", req.Username, req.Email).First(&existing).Error; err == nil {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "用户已存在")
		return
	}
	// hash 密码
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "密码处理失败")
		return
	}
	user := models.User{Username: req.Username, Email: req.Email, Password: hash}
	if err := db.Global.Create(&user).Error; err != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "创建失败")
		return
	}
	ttl := appConfig.Auth.AccessTTL
	if ttl == 0 {
		ttl = 24 * 3600 * 1e9
	}
	token, err := auth.GenerateToken(user.ID, user.Username, ttl)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "token 生成失败")
		return
	}
	c.JSON(http.StatusOK, AuthResponse{Code: 0, Msg: "ok", Data: AuthData{Token: token, UserID: user.ID}})
}

// handleLogin 支持用用户名或邮箱登录
// handleLogin 用户登录
// @Summary 用户登录
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body loginReq true "登录参数 (username 或 email 任选其一)"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} BadRequestError
// @Failure 500 {object} InternalModelCallError
// @Router /api/auth/login [post]
func handleLogin(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "参数错误")
		return
	}
	if req.Username == "" && req.Email == "" {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "username 或 email 必填其一")
		return
	}
	var user models.User
	q := db.Global
	if req.Username != "" {
		q = q.Where("username = ?", req.Username)
	} else {
		q = q.Where("email = ?", req.Email)
	}
	if err := q.First(&user).Error; err != nil {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "用户不存在")
		return
	}
	if !auth.CheckPassword(user.Password, req.Password) {
		writeErr(c, http.StatusBadRequest, CodeBadParam, "密码错误")
		return
	}
	ttl := appConfig.Auth.AccessTTL
	if ttl == 0 {
		ttl = 24 * 3600 * 1e9
	}
	token, err := auth.GenerateToken(user.ID, user.Username, ttl)
	if err != nil {
		writeErr(c, http.StatusInternalServerError, CodeInternalError, "token 生成失败")
		return
	}
	c.JSON(http.StatusOK, AuthResponse{Code: 0, Msg: "ok", Data: AuthData{Token: token, UserID: user.ID}})
}
