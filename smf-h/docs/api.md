# API 文档 (Swagger 注释版同步)

> 本文件为手写说明，实际以 /swagger/index.html 展示的生成文档为准。生成工具： swag (github.com/swaggo/swag)

## 认证
所有受保护接口需要 Header: `Authorization: Bearer <token>`

## 错误码
| code | 含义 |
|------|------|
| 0 | 成功 |
| 40001 | 参数错误 |
| 40002 | 模型不存在或未配置 |
| 40101 | 未认证/Token 无效 |
| 40301 | 模型权限不足（等级不够） |
| 40302 | 会话不属于当前用户 |
| 50000 | 服务内部错误 |
| 50001 | 模型返回为空 |

### 错误结构映射
所有错误均统一 JSON 结构 `{ "code": <int>, "msg": <string> }`，但在 Swagger 中以不同结构体名区分业务语义：

| 结构体 | 典型 code | 说明 | 示例 `msg` |
|--------|-----------|------|-----------|
| BadRequestError | 40001 | 通用参数校验失败 | `参数错误: question 必填` |
| ModelNotFoundError | 40002 | 模型 ID 不存在或未在策略中配置 | `模型不存在或未配置` |
| UnauthorizedError | 40101 | 缺少或非法的 Bearer Token | `未提供或非法的 Authorization 头` |
| ForbiddenModelError | 40301 | 用户等级不满足模型最小等级 | `无权限使用模型(doubao...)，需要等级≥3` |
| ForbiddenConversationError | 40302 | 访问他人会话 | `会话不属于当前用户` |
| InternalModelCallError | 50000 | 调用模型/数据库等内部错误 | `模型调用失败` |
| InternalEmptyReturnError | 50001 | 模型响应结构没有有效内容 | `模型返回为空` |

> Swagger 会列出同一 HTTP 状态码下的多种错误结构（例如多个 400/403/500），便于前端生成精确的错误分支。

## 接口列表
### 健康检查
GET /health

### 注册
POST /api/auth/register
Body:
```
{
  "username": "u1",
  "email": "u1@test.com",
  "password": "Abcd1234!"
}
```
返回:
```
{ "code":0, "data": {"token":"...", "user_id":1} }
```

### 登录
POST /api/auth/login
Body 支持 username 或 email:
```
{ "username":"u1", "password":"Abcd1234!" }
```

### 发送消息
POST /api/chat/send (需登录)
```
{
  "conversation_id": "",  // 为空即新建
  "question": "你好",
  "model": "doubao-seed-1-6-250615"
}
```
成功:
```
{
  "code":0,
  "data":{
    "conversation_id":"uuid",
    "answer":"...",
    "used_history":2
  }
}
```
错误示例：
- 模型不存在: `{ "code":40002, "msg":"模型不存在或未配置" }`
- 等级不足: `{ "code":40301, "msg":"无权限使用模型(doubao...)，需要等级≥3" }`
- 会话不属于用户: `{ "code":40302, "msg":"会话不属于当前用户" }`

### 获取历史
GET /api/chat/history?conversation_id=xxx (需登录)
返回:
```
{
  "code":0,
  "data": {
    "conversation_id":"xxx",
    "messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}],
    "count":2
  }
}
```

### 清空 / 删除会话
POST /api/chat/clear (需登录)
```
{
  "conversation_id": "xxx",
  "full": false   // 可选，true=连同会话元数据彻底删除
}
```
响应示例：
```
{
  "code":0,
  "data": {
    "conversation_id":"xxx",
    "deleted": false,   // full=true 且会话记录删除后为 true
    "full": false,
    "message_deleted": 4,  // 本次实际删除的消息条数
    "conversation_deleted": 1 // full=true 且会话记录删除后=1，否则缺省或=0
  }
}
```
行为说明：
| full | 动作 |
|------|------|
| false | 删除该会话全部消息，保留 conversations 记录 (可继续追加历史) |
| true  | 删除消息 + 会话记录 + 内存条目 + Redis 缓存键 |

Redis 缓存键：`chat:conv:recent:<id>` 会被同时删除。

### 会话列表
GET /api/chat/list (需登录)
```
返回:
{
  "code":0,
  "data": {
    "conversations": [
       {"id":"c1","last_active_at":"2025-10-08T10:11:12Z"},
       {"id":"c2","last_active_at":"2025-10-07T09:01:00Z"}
    ],
    "count": 2
  }
}
```
当前未分页，可后续加 `?limit=&offset=`。

### 调试查看缓存 (仅 DEBUG 用)
GET /api/chat/debug/cache?conversation_id=xxx (需登录 & 环境变量 `DEBUG_CACHE=1` 才启用)
```
返回:
{
  "code":0,
  "data":{
    "conversation_id":"xxx",
    "cached_recent":"[ {..}, ... ]",
    "length": 456   // JSON 原始字符串长度
  }
}
```
若未开启返回 403。

## 计划中 (Planned)
- POST /api/chat/send/stream  SSE 流式输出
- 分页的会话列表 (limit / offset)
- 会话摘要 (summary) 与多级上下文压缩
- Token 精确统计（替换粗略 roughTokenEstimate）

## 会话上下文与缓存
当前 /send 在内部会：
1. 若内存中该会话为空 -> 先 warm：尝试 Redis 最近消息缓存 -> 不命中则 DB 拉取全部（随后内存 FIFO 剪裁）。
2. 取最近 `2 * max_history` 条消息作为上下文（扩大窗口以提高回答连续性）。
3. 写入用户消息、调用模型、写入助手消息。
4. 将最近 `max_history*2` 消息写入 Redis (`chat:conv:recent:<id>` TTL 10m)。

清空 `/clear`：
- full=false 删除 messages + 内存数组 + 缓存 recent
- full=true 还会删除 conversations 记录，并从内存管理器移除整个会话

## Swagger 使用
安装 swag CLI 本地生成:
```
swag init --parseDependency --parseInternal -g main.go -o docs/swagger
```
访问: http://localhost:8080/swagger/index.html

## Changelog
- v0.4 新增：模型权限、会话/消息持久化、错误码 40002/40302，细化错误结构（BadRequestError / ModelNotFoundError / Forbidden* / Internal*）并统一 writeErr 输出
