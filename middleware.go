package reqlogmid

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

// contextKey 用于在 gin.Context 中存储日志条目
type contextKey string

const logEntryKey contextKey = "req_log_entry"

// RequestLogger 创建并返回请求日志中间件
// logger 日志输出器实例
func RequestLogger(logger Logger) gin.HandlerFunc {
	return RequestLoggerWithConfig(logger, DefaultConfig())
}

// RequestLoggerWithConfig 创建并返回带配置的请求日志中间件
// logger 日志输出器实例
// cfg 配置选项
func RequestLoggerWithConfig(logger Logger, cfg *Config) gin.HandlerFunc {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 确保配置有效
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = DefaultTimeFormat
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}

	// 返回中间件处理函数
	return func(c *gin.Context) {
		// 每次请求时读取最新配置（使用读锁保护）
		cfg.RLock()
		isEnabled := cfg.Enabled
		skipPaths := cfg.SkipPaths
		customFields := cfg.CustomFields
		async := cfg.Async
		timeFormat := cfg.TimeFormat
		cfg.RUnlock()

		// 检查是否启用
		if !isEnabled {
			c.Next()
			return
		}

		// 检查是否跳过当前路径
		if len(skipPaths) > 0 {
			currentPath := c.Request.URL.Path
			for _, sp := range skipPaths {
				if currentPath == sp {
					c.Next()
					return
				}
			}
		}

		// 记录请求开始时间
		startTime := time.Now()

		// 保存原始请求体，用于读取后再次获取
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 处理请求
		c.Next()

		// 计算处理耗时
		duration := time.Since(startTime)

		// 重新填充请求体（因为中间件可能已经读取过）
		if len(bodyBytes) > 0 {
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 获取响应状态码
		statusCode := c.Writer.Status()

		// 获取客户端IP
		clientIP := c.ClientIP()

		// 获取 User-Agent
		userAgent := c.Request.UserAgent()

		// 创建日志条目
		entry := NewLogEntry(
			c.Request.Method,
			c.Request.URL.Path,
			clientIP,
			userAgent,
			statusCode,
			duration,
			time.Now().Format(timeFormat),
		)

		// 添加自定义字段
		if customFields != nil {
			entry.CustomFields = make(map[string]interface{}, len(customFields)+1)
			for k, v := range customFields {
				entry.CustomFields[k] = v
			}
		}

		// 将日志条目存储到上下文中，供后续处理使用
		c.Set(string(logEntryKey), entry)

		if async {
			go func() {
				// 复制一份日志条目，避免并发访问问题
				logCopy := *entry
				if err := logger.Write(&logCopy); err != nil {
					_ = err
				}
			}()
		} else {
			if err := logger.Write(entry); err != nil {
				_ = err
			}
		}
	}
}

// GetLogEntry 从 gin.Context 中获取日志条目
func GetLogEntry(c *gin.Context) *LogEntry {
	if entry, exists := c.Get(string(logEntryKey)); exists {
		if logEntry, ok := entry.(*LogEntry); ok {
			return logEntry
		}
	}
	return nil
}

// SetLogField 向当前请求的日志条目中添加自定义字段
func SetLogField(c *gin.Context, key string, value interface{}) {
	if entry := GetLogEntry(c); entry != nil {
		if entry.CustomFields == nil {
			entry.CustomFields = make(map[string]interface{})
		}
		entry.CustomFields[key] = value
	}
}
