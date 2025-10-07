# AI Chat Service

一个包含用户认证、模型权限控制、会话持久化、历史缓存与 Swagger 文档的简化 AI 多轮对话示例服务。

## 功能特性
- 用户注册 / 登录（JWT，环境变量可覆盖密钥）
- 模型访问权限控制（用户等级 / 角色 + 模型最小等级）
- 会话与消息：MySQL 持久化 + 内存缓存 + 最近历史 Redis 缓存
- 统一错误码与结构化响应（支持多类错误 swagger 展示）
- Swagger 文档（动态生成）
- 环境变量安全覆写配置，真实 `config.yaml` 不提交
- Redis 可选：失败自动降级不影响主流程

## 快速启动 (Windows PowerShell)
```powershell
# 可选：克隆后进入目录
# git clone <your-fork-url> && cd awesomeProject

# 1. 准备示例配置（真实敏感值用环境变量覆盖）
Copy-Item config\config.example.yaml config\config.yaml

# 2. 设置必要环境变量（示例，可按需调整）
$env:MYSQL_PASSWORD = "your_mysql_password"
$env:AUTH_JWT_SECRET = "your_jwt_secret"
# 如果需要真实模型调用
# $env:ARK_API_KEY = "your_model_api_key"

# 3. 启动服务
go run main.go

# 4. 打开 Swagger
# http://localhost:8080/swagger/index.html
```

## 错误码对照（节选）
| code | 含义 |
|------|------|
| 0 | 成功 |
| 40001 | 参数错误 |
| 40002 | 模型不存在或未配置 |
| 40101 | 未认证 |
| 40301 | 模型权限不足 |
| 40302 | 会话归属错误 |
| 50000 | 内部错误 |
| 50001 | 模型返回为空 |

完整说明参见：`docs/api.md`

## 目录说明
```
internal/            内部逻辑模块
  auth/              JWT、密码哈希、中间件
  config/            配置加载与环境变量覆写
  db/                数据库初始化
  memory/            内存会话管理
  models/            GORM 实体
  redisstore/        Redis 可选缓存封装
config/              配置文件目录（示例 + 本地实际）
docs/                架构/接口/安全/代码说明文档
main.go              程序入口与路由
```

## 常见环境变量
```
MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DB / MYSQL_CHARSET
AUTH_JWT_SECRET / AUTH_ACCESS_TTL
CHAT_DEFAULT_MODEL / CHAT_MAX_HISTORY / CHAT_MEMORY_LIMIT_MSGS
REDIS_HOST / REDIS_PORT / REDIS_PASSWORD / REDIS_DB
```
详细列表：`docs/SECURITY_CONFIG.md`

## Redis 最近历史缓存
- Key: `chat:conv:recent:<conversation_id>`
- 写入：发送消息成功后
- 读取：历史接口优先命中；失败走 DB 回源
- 降级：Redis 初始化失败时自动忽略缓存

## 后续可扩展
- SSE 流式输出
- 速率限制 (Redis Incr)
- 会话列表接口
- 模型 Provider 抽象层

## PR / 评审建议
- 不要提交 `config/config.yaml`
- 通过 README 步骤可本地快速运行
- 安全策略见 `docs/SECURITY_CONFIG.md`

## License
按上游仓库策略或后续补充。

---
**END**
