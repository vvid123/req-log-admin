# req-log-mid

Go Gin HTTP 请求日志中间件，支持文件/数据库存储，提供管理界面。

## 特性

- **多种存储**：支持文件、PostgreSQL、MySQL、SQLite
- **数据库存储**：日志持久化到数据库
- **管理界面**：Web 界面查看日志、修改配置
- **配置持久化**：配置存储在数据库
- **高性能**：异步写入，不阻塞请求
- **热更新**：配置修改立即生效

## 安装

```bash
go get github.com/zxyao/req-log-mid
```

## 依赖

```bash
# PostgreSQL（推荐）
go get github.com/lib/pq

# MySQL
go get github.com/go-sql-driver/mysql

# SQLite
go get github.com/mattn/go-sqlite3
```

## 快速开始

### 文件存储

```go
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"req-log-mid"
)

func main() {
	// 创建文件日志输出器
	logger, err := reqlogmid.NewFileLogger("access.log", true, 1000)
	if err != nil {
		panic(err)
	}
	defer logger.Close()

	// 配置中间件
	cfg := reqlogmid.DefaultConfig()
	cfg.Enabled = true
	cfg.SkipPaths = []string{"/health"}

	r := gin.Default()
	r.Use(reqlogmid.RequestLoggerWithConfig(logger, cfg))
	r.GET("/hello", func(c *gin.Context) {
		c.String(200, "Hello!")
	})

	// 优雅关闭
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Flush()
	}()

	r.Run(":8080")
}
```

### 数据库存储（推荐）

```go
package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"req-log-mid"
	"req-log-mid/admin"
)

func main() {
	// 创建数据库日志器
	logger, err := reqlogmid.NewDBLogger(
		"postgres",
		"host=localhost user=postgres password=123456 dbname=mylog sslmode=disable",
		true, 1000,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Close()

	// 创建日志表
	logger.CreateTable()

	// 初始化配置仓储
	configRepo := admin.NewConfigRepository(logger.DB())
	configRepo.InitConfigTable()

	// 从数据库加载配置
	dbConfig, _ := configRepo.LoadConfig()
	cfg := reqlogmid.DefaultConfig()
	cfg.Enabled = dbConfig.Enabled
	cfg.Async = dbConfig.AsyncMode
	cfg.SkipPaths = admin.ParseSkipPaths(dbConfig.SkipPaths)

	r := gin.Default()
	r.Use(reqlogmid.RequestLoggerWithConfig(logger, cfg))

	// 注册管理 API
	admin.RegisterRoutes(r,
		admin.NewLogAdminHandler(logger),
		admin.NewConfigAdminHandler(configRepo, cfg),
	)

	r.Run(":8080")
}
```

## 管理界面

访问 **http://localhost:8080/admin**

功能：
- 查看请求日志列表
- 筛选、搜索日志
- 实时配置开关
- 配置持久化存储

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/logs` | 日志列表（支持分页、筛选） |
| GET | `/admin/logs/:id` | 日志详情 |
| DELETE | `/admin/logs?days=7` | 清理旧日志 |
| GET | `/admin/config` | 获取配置 |
| PUT | `/admin/config` | 更新配置 |
| POST | `/admin/config/reset` | 重置配置 |
| GET | `/admin/stats` | 统计数据 |
| GET | `/admin/health` | 健康检查 |

### 更新配置示例

```bash
# 启用日志
curl -X PUT http://localhost:8080/admin/config \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "async": true, "buffer_size": 1000}'
```

## 配置说明

| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `Enabled` | bool | 中间件开关 | `true` |
| `SkipPaths` | []string | 跳过记录的路径 | `["/health","/metrics"]` |
| `CustomFields` | map | 自定义日志字段 | `nil` |
| `Async` | bool | 异步写日志 | `true` |
| `BufferSize` | int | 异步缓冲区大小 | `1000` |

## 日志格式

```json
{
    "id": 1,
    "method": "GET",
    "path": "/api/users",
    "client_ip": "192.168.1.100",
    "user_agent": "Mozilla/5.0",
    "status_code": 200,
    "duration_ms": 15.5,
    "timestamp": "2026-02-14T10:30:00.123Z",
    "custom_fields": {"user_id": "12345"},
    "created_at": "2026-02-14 10:30:00"
}
```

## 文件结构

```
req-log-mid/
├── middleware.go      # 中间件主逻辑
├── logger.go         # Logger 接口和 LogEntry 定义
├── file_logger.go    # 文件输出实现
├── db_logger.go      # 数据库输出实现
├── config.go         # 配置结构体
├── admin/
│   ├── handler.go     # 管理 API 处理器
│   ├── config_repo.go # 配置仓储
│   └── index.html    # 管理界面
├── database/
│   └── migrations/
│       └── 001_init.sql  # 数据库迁移脚本
└── example/
    ├── main.go       # 文件存储示例
    └── db/main.go   # 数据库存储示例
```

## 数据库表结构

### request_logs - 请求日志表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL | 主键 |
| method | VARCHAR(10) | HTTP 方法 |
| path | VARCHAR(512) | 请求路径 |
| client_ip | VARCHAR(45) | 客户端 IP |
| user_agent | VARCHAR(512) | User-Agent |
| status_code | INT | 状态码 |
| duration_ms | DOUBLE | 响应时间(ms) |
| timestamp | VARCHAR(32) | 时间戳 |
| custom_fields | JSONB | 自定义字段 |
| created_at | TIMESTAMP | 创建时间 |

### log_config - 配置表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INT | 主键 |
| enabled | BOOLEAN | 是否启用 |
| async_mode | BOOLEAN | 异步模式 |
| buffer_size | INT | 缓冲区大小 |
| skip_paths | TEXT | 跳过路径(逗号分隔) |
| custom_fields | JSONB | 自定义字段 |
| updated_at | TIMESTAMP | 更新时间 |

## 集成到现有项目

```bash
# 添加依赖
go get github.com/zxyao/req-log-mid@latest
```

```go
import (
	"github.com/gin-gonic/gin"
	"req-log-mid"
	"req-log-mid/admin"
)

func main() {
	// 创建数据库日志器
	logger, _ := reqlogmid.NewDBLogger("postgres", "dsn", true, 1000)
	logger.CreateTable()

	// 配置仓储
	configRepo := admin.NewConfigRepository(logger.DB())
	configRepo.InitConfigTable()

	// 加载配置
	cfg := reqlogmid.DefaultConfig()
	if dbCfg, err := configRepo.LoadConfig(); err == nil {
		cfg.Enabled = dbCfg.Enabled
		cfg.SkipPaths = admin.ParseSkipPaths(dbCfg.SkipPaths)
	}

	r := gin.Default()
	r.Use(reqlogmid.RequestLoggerWithConfig(logger, cfg))

	// 管理路由
	admin.RegisterRoutes(r,
		admin.NewLogAdminHandler(logger),
		admin.NewConfigAdminHandler(configRepo, cfg),
	)

	r.Run(":8080")
}
```

## 许可证

MIT
