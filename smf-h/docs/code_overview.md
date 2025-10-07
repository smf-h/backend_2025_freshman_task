# 代码说明文档 (Code Overview)

> 面向开发者的代码级说明，聚焦目录结构、关键流程、扩展点与最佳实践。与 `architecture.md`（宏观架构）和 `api.md`（接口）配套阅读。

---
## 1. 项目概览
本项目是一个简化版 AI 问答 / Chat 系统服务端，提供：
- 用户注册 / 登录（JWT）
- 模型访问权限控制（按用户等级 / 角色）
- 会话与消息持久化（数据库 + 内存增量缓存）
- 多轮对话上下文裁剪（最近 N 条 / Token 粗估）
- 统一错误码与结构化响应
- Swagger 文档生成

运行方式（需 Go 1.24+）：
```powershell
# (可选) 设置模型 API Key
$env:ARK_API_KEY = "<your-api-key>"
# 启动
go run main.go
# 访问 Swagger
http://localhost:8080/swagger/index.html
```

---
## 2. 目录结构说明
```
.
├── main.go                  # 入口：路由、依赖初始化、Handler
├── go.mod / go.sum          # 依赖管理
├── config/                  # 配置文件 (config.yaml)
├── internal/
│   ├── auth/                # 认证与权限：JWT、密码、middleware
│   ├── config/              # 配置加载解析 (YAML -> struct)
│   ├── db/                  # GORM 初始化与全局句柄
│   ├── memory/              # 内存会话管理器 (MemoryManager)
│   └── models/              # 数据表模型 (User / Conversation / Message)
└── docs/
    ├── api.md               # 手写接口说明
    ├── architecture.md      # 架构设计文档
    ├── code_overview.md     # (本文件)
    └── swagger/             # swag 自动生成的 swagger 规范文件
```

### internal 分层意图
| 包 | 职责 | 依赖方向 |
|----|------|----------|
| auth | JWT 解析/生成、密码哈希、鉴权中间件 | 仅依赖标准库 & 第三方库 |
| config | 加载 YAML -> Root 配置结构 | 无业务依赖 |
| db | 初始化全局 `db.Global` (GORM) | 依赖配置 | 
| memory | 纯内存会话上下文存储 | 无数据库依赖 | 
| models | 定义 GORM 实体 | 依赖 GORM | 
| main.go | 组装应用：路由、处理流程 | 依赖以上所有 |

---
## 3. 配置系统 (config)
配置文件示例（`config/config.yaml`）：
```yaml
server:
  port: 8080
  mode: release
mysql:
  user: root
  password: 123456
  host: 127.0.0.1
  port: 3306
  database: chatdb
  charset: utf8mb4
chat:
  default_model: doubao-seed-1-6-250615
  max_history: 6
  memory_limit_msgs: 50
auth:
  jwt_secret: "dev-secret"
  access_ttl: 86400000000000   # 24h 纳秒值
models:
  - id: doubao-seed-1-6-250615
    min_level: 1
  - id: doubao-pro-x
    min_level: 3
```
加载逻辑：`config.Load(path)` -> 反序列化 -> `main.go` 存入全局 `appConfig`。

---
## 4. 数据模型 (models)
| 表 | 关键字段 | 说明 |
|----|----------|------|
| users | username/email/password/level/role | 角色扩展未来用于管理端 |
| conversations | id(user provided uuid)/user_id/last_active_at | 会话元数据，用于归属与排序 |
| messages | conversation_id/user_id/role/content/token_count | 原始对话消息记录 |

注意：
- `conversation_id` 为外部可见 ID（UUID）。
- 历史消息内存缓存只保留最近若干条，数据库持久化全量，用于历史回放。

---
## 5. 认证与权限 (auth)
流程：
1. 注册：bcrypt 加密密码 -> 写入用户 -> 生成 JWT。
2. 登录：按用户名或邮箱匹配 -> 校验密码 -> 发放 JWT。
3. 中间件：从 Header `Authorization: Bearer <token>` 解析，放入 `Context`（userID/level/role）。
4. 模型访问：在 `handleChatSend` 中根据 `modelLevelMap[modelID]` 与用户 level 校验；管理员角色可跳过（`HasAdminRole`）。

JWT 声明包含：用户 ID、用户名、过期时间；HS256 签名，密钥来自配置 `auth.jwt_secret`。

---
## 6. 内存会话 (memory)
核心结构：
- `MemoryManager`：`map[conversationID]*ConversationMemory`
- `ConversationMemory`：`[]Message`（环或截断策略）

策略：
- 每次追加消息时估算 token（`roughTokenEstimate` 基于分词近似 *1.3 乘子）。
- 获取模型输入时使用 `LastN(maxHistoryToUse)` 控制上下文长度。
- 如果进程重启，内存上下文清空，但前端可通过 `/history` 重新灌入（当前实现为首次访问历史时只向空内存填充）。

---
## 7. Handler 关键流程
### 7.1 发送消息 `/api/chat/send`
```
[Bind JSON]
  -> (可生成 conversation_id & DB 创建会话)
  -> 会话归属校验 (conversation.user_id == token.user_id)
  -> 模型存在 & 权限等级校验
  -> 内存追加用户消息 + DB 保存
  -> 截取最近 N 条历史 => 调用模型 SDK
  -> 校验返回结构 (choices / content)
  -> 追加助手消息 (内存 + DB)
  -> 返回 answer / used_history
```

### 7.2 获取历史 `/api/chat/history`
```
[Query conversation_id]
  -> 归属校验
  -> DB 全量读取 messages 按 id 升序
  -> 内存若为空则灌入（用于后续继续对话）
  -> 返回 messages 列表
```

### 7.3 清空会话 `/api/chat/clear`
```
[Bind conversation_id]
  -> 归属校验
  -> DB 删除该会话所有 messages
  -> 内存 Clear()
  -> 返回成功
```

### 7.4 注册 / 登录
注册：查重 -> hash 密码 -> 写库 -> 生成 token。  
登录：按 username/email 选路 -> 查库 -> 校验密码 -> 生成 token。

---
## 8. 错误处理
统一入口：`writeErr(c, httpStatus, code, msg)` -> `{ code, msg }`。  
错误码定义集中在 `main.go`：
```
const (
  CodeOK = 0
  CodeBadParam = 40001
  CodeModelNotFound = 40002
  CodeUnauthorized = 40101
  CodeForbiddenModel = 40301
  CodeForbiddenConversation = 40302
  CodeInternalError = 50000
  CodeModelEmpty = 50001
)
```
Swagger 通过不同结构体名（`BadRequestError` 等）让前端区分分支。

---
## 9. 模型调用
使用字节火山方舟 SDK：
- Client 初始化：`arkruntime.NewClientWithApiKey(apiKey, WithBaseUrl(...))`
- 调用：`client.CreateChatCompletion(ctx, model.CreateChatCompletionRequest{Model: req.Model, Messages: modelMsgs})`
- 将内存消息转换为 SDK 消息：`convertToModelMessages`。

扩展其他模型：可封装接口 `ModelProvider`，当前代码为内联调用，可后续抽象：
```
type ModelProvider interface { Chat(messages []ChatMessage) (answer string, err error) }
```

---
## 10. Swagger 文档
- 注释写在 `main.go` handler 前。
- 生成命令：
```powershell
swag init --parseDependency --parseInternal -g main.go -o docs/swagger
```
- 访问：`/swagger/index.html`
- 注意：多个相同状态码 @Failure 会分别生成；某些 UI 可能只默认展示首个，需要说明。

---
## 11. 扩展指南
### 11.1 新增模型策略
1. 在 `config.yaml` 增加：
```yaml
models:
  - id: new-model-x
    min_level: 2
```
2. 重启服务 -> `modelLevelMap` 自动加载。

### 11.2 新增会话列表接口
- 模型：直接查询 `conversations` where `user_id = ?` order by `last_active_at desc` limit N。
- 返回最近会话基本信息（id, last_active_at, message_count）。

### 11.3 添加流式输出 (SSE)
- 新路由：`/api/chat/send/stream`
- Header：`Content-Type: text/event-stream`
- 逐步写入 `data: <partial>` 并 `flush`
- 仍然需保存完整最终消息到 DB / 内存。

### 11.4 抽象模型层
创建 `internal/llm/`：
```
type ChatMessage struct { Role, Content string }
interface Provider { Complete(ctx context.Context, msgs []ChatMessage) (string, error) }
```
主 handler 仅依赖接口 -> 支持多实现切换。

### 11.5 引入缓存/限流
- 限流：中间件令牌桶（如 golang.org/x/time/rate）。
- 模型调用缓存：key=(model,hash(latestN))，短时相同问题加速。

### 11.6 用户角色拓展
- 增加 `role=admin` 权限：可列出所有用户会话 / 强制清除。
- 编写基于 `role` 的路由组中间件。

---
## 12. 测试建议
| 测试类型 | 重点 |
|----------|------|
| 单元 | memory 管理 append / trim；密码哈希校验；权限判断逻辑 |
| 集成 | 注册->登录->发消息->历史->清空 全链路 |
| 异常 | 无 token / 等级不足 / 非本人会话 / 模型不存在 |
| 回归 | 错误码与文档一致性 |

可以引入 `httptest`：
```go
w := httptest.NewRecorder()
req, _ := http.NewRequest("POST", "/api/chat/send", bytes.NewReader(body))
req.Header.Set("Authorization", "Bearer <token>")
router.ServeHTTP(w, req)
```

---
## 13. 运维与部署
| 关注点 | 建议 |
|--------|------|
| 配置 | 区分 dev / prod，多套 yaml 或注入 ENV 覆盖关键项 (DB, JWT Secret, API Key) |
| 日志 | 目前使用默认 Gin；生产建议接入结构化日志 (zap/logrus) |
| 迁移 | 目前 AutoMigrate；复杂场景使用 goose / atlas | 
| 安全 | 强制 HTTPS、JWT Secret 定期轮换、限制密码尝试次数 |
| 监控 | 增加 /metrics (Prometheus) & 慢查询日志 |

---
## 14. 常见问题 (FAQ)
| 问题 | 原因 | 解决 |
|------|------|------|
| Swagger `/swagger/doc.json` 500 | 缺少空白导入或重复 `docs.go` | 保留 `_ ".../docs/swagger"` 且清理重复生成 | 
| 模型权限未生效 | 配置未加载/模型ID拼写 | 检查 `models` 配置与日志 | 
| 历史上下文丢失 | 进程重启内存清空 | 通过 `/history` 首次加载再继续聊天 |
| Token 过期过快 | access_ttl 配置单位为纳秒 | 确认 YAML 数值是否过小 |

---
## 15. 后续可演进方向
- 模型多实例 + 超时/重试封装
- 分布式会话（Redis 替内存）
- 统一审计日志 & 操作追踪 ID
- 细化用户等级成长机制（积分/调用次数）
- OpenAPI 代码生成前端 SDK

---
## 16. 快速清单 (Checklist)
| 事项 | 位置 | 说明 |
|------|------|------|
| 添加新错误码 | `main.go` 常量区 | 同步 `api.md` & `code_overview.md` |
| 新增 Handler | `main.go` | 增加 Swagger 注释 + 使用 `writeErr` |
| 新增模型策略 | `config.yaml` | 重启即可生效 |
| 数据库字段变更 | `models/` | 执行 AutoMigrate (开发) / 正式使用迁移工具 |
| Token 问题调试 | `internal/auth` | 检查 TTL / Secret | 

---
**END**
