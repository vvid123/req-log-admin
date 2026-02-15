package reqlogmid

import "sync"

// DefaultTimeFormat 默认时间格式
const DefaultTimeFormat = "2006-01-02T15:04:05.000Z07:00"

// Config 定义中间件的配置选项
type Config struct {
	sync.RWMutex
	// Enabled 控制中间件是否启用
	Enabled bool
	// SkipPaths 跳过记录指定路径
	SkipPaths []string
	// CustomFields 自定义字段，会添加到每条日志中
	CustomFields map[string]interface{}
	// TimeFormat 时间格式，默认使用 RFC3339Milli
	TimeFormat string
	// Async 是否异步写日志，默认 true
	Async bool
	// BufferSize 异步日志缓冲区大小，默认 1000
	BufferSize int
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:      true,
		SkipPaths:    []string{"/health", "/metrics", "/.well-known/appspecific/com.chrome.devtools.json"},
		CustomFields: nil,
		TimeFormat:   DefaultTimeFormat,
		Async:        true,
		BufferSize:   1000,
	}
}
