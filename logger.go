package reqlogmid

import (
	"encoding/json"
	"time"
)

// LogEntry 表示一条日志条目
type LogEntry struct {
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	ClientIP     string                 `json:"client_ip"`
	UserAgent    string                 `json:"user_agent"`
	StatusCode   int                    `json:"status_code"`
	Duration     float64                `json:"duration_ms"`
	Timestamp    string                 `json:"timestamp"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

// Logger 接口定义了日志输出的抽象
// 任何实现了此接口的类型都可以作为日志输出器
type Logger interface {
	// Write 将日志条目写入输出
	Write(entry *LogEntry) error
	// Close 关闭日志输出器，释放资源
	Close() error
	// Flush 刷新所有待写入的日志（用于异步日志）
	Flush()
}

// MarshalJSON 实现 json.Marshaler 接口
func (l *LogEntry) MarshalJSON() ([]byte, error) {
	type Alias LogEntry
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(l),
	})
}

// NewLogEntry 创建一个新的日志条目
func NewLogEntry(method, path, clientIP, userAgent string, statusCode int, duration time.Duration, timestamp string) *LogEntry {
	return &LogEntry{
		Method:     method,
		Path:       path,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
		StatusCode: statusCode,
		Duration:   float64(duration) / float64(time.Millisecond),
		Timestamp:  timestamp,
	}
}
