# 配置与密钥安全指南

本指南说明如何在提交代码(PR)时避免泄露敏感信息，并保证不同环境(本地/测试/生产)配置隔离。

## 1. 不要提交的内容
- 真实 `config/config.yaml` (已加入 .gitignore)
- 数据库密码、JWT 密钥、第三方 API Key (如 ARK_API_KEY)
- 生产 Redis / MySQL 地址
- 私有证书、密钥对、令牌文件

## 2. 提交什么
| 文件 | 作用 | 是否包含敏感信息 |
|------|------|------------------|
| `config/config.example.yaml` | 示例/模板 | 否(占位符) |
| `internal/config/config.go` | 解析 + 环境变量覆盖 | 否 |
| `.gitignore` | 忽略真实配置 | 否 |

## 3. 本地真实配置放哪里
- 开发: `config/config.yaml` (不会被 git 追踪)
- 生产: 推荐使用环境变量 + 仅最小必要的 fallback config 文件

## 4. 环境变量覆盖支持 (已实现)
支持的变量 (非空即覆盖)：
```
SERVER_PORT, SERVER_MODE
MYSQL_HOST, MYSQL_PORT, MYSQL_USER, MYSQL_PASSWORD, MYSQL_DB, MYSQL_CHARSET
AUTH_JWT_SECRET, AUTH_ACCESS_TTL
CHAT_DEFAULT_MODEL, CHAT_MAX_HISTORY, CHAT_MEMORY_LIMIT_MSGS, CHAT_REQUEST_TIMEOUT
REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_DB, REDIS_POOL_SIZE,
REDIS_DIAL_TIMEOUT, REDIS_READ_TIMEOUT, REDIS_WRITE_TIMEOUT
```
示例 (PowerShell 临时会话)：
```powershell
$env:MYSQL_PASSWORD = "SuperSecret!"
$env:AUTH_JWT_SECRET = "ProdJwtSecret_ChangeMe"
$env:REDIS_PASSWORD = "RedisPwd123"
```

## 5. PR 前自检清单
| 检查项 | 通过条件 |
|--------|----------|
| config.yaml 是否未被提交 | `git status` 无该文件 |
| 未把真实密码硬编码进代码 | 搜索 `password:` / `jwt_secret:` 无真实值 |
| 关键密钥均可用 env 注入 | 启动服务时通过日志/调试确认被覆盖 |

## 6. CI/CD 建议
- 使用平台 Secret 管理 (GitHub Actions Secrets / GitLab CI Variables)
- 部署启动脚本导出变量后再 `go run / build`：
```bash
export MYSQL_PASSWORD=$SECRET_MYSQL_PASSWORD
export AUTH_JWT_SECRET=$SECRET_JWT
./app
```
- 避免将密钥写入镜像层：用运行时注入 (Kubernetes Secret / Docker --env-file)

## 7. 生产加固建议
| 领域 | 建议 |
|------|------|
| JWT | 定期轮换密钥，考虑多个版本(旧+新)并行过渡 |
| 数据库 | 最小权限账号，禁止 *.* 全权限；开启 TLS (如需) |
| Redis | 绑定内网地址，启用 ACL 或至少强密码，禁止公网暴露 |
| 日志 | 不打印 token / 密码 / 完整 SQL (参数可脱敏) |
| 代码 | 通过 secret 扫描工具 (trufflehog / gitleaks) 预防泄露 |

## 8. 临时调试不泄露技巧
- 打印配置时跳过敏感字段 (`****`)。
- 将敏感值长度或 hash 输出用于核对是否加载到了正确的 Secret。

## 9. 应急处理
| 场景 | 动作 |
|------|------|
| 不小心提交密钥 | 立即在平台侧作废该密钥；强制推送清理历史(必要时)；生成新 Key |
| 密钥疑似泄露 | 强制失效 + 全局通知 + 轮换；审计访问日志 |
| 配置被恶意篡改 | 使用只读挂载或配置签名校验 |

## 10. 后续可增强
- 引入 `.env` + dotenv 解析（本项目目前依赖系统 env）
- 加入启动时敏感配置缺失校验并统一错误提示
- 敏感值自动检测 pre-commit 钩子
- 支持 Vault / Secrets Manager 远程加载

---
**END**
