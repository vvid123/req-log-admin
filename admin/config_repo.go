package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DBConfig 数据库配置模型
type DBConfig struct {
	ID           int       `json:"id"`
	Enabled      bool      `json:"enabled"`
	AsyncMode    bool      `json:"async_mode"`
	BufferSize   int       `json:"buffer_size"`
	SkipPaths    string    `json:"skip_paths"`
	CustomFields string    `json:"custom_fields"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ConfigRepository 配置仓储
type ConfigRepository struct {
	db        *sql.DB
	tableName string
}

// NewConfigRepository 创建配置仓储
func NewConfigRepository(db *sql.DB) *ConfigRepository {
	return &ConfigRepository{
		db:        db,
		tableName: "log_config",
	}
}

// LoadConfig 从数据库加载配置
func (r *ConfigRepository) LoadConfig() (*DBConfig, error) {
	query := fmt.Sprintf(`
		SELECT id, enabled, async_mode, buffer_size,
		       COALESCE(skip_paths, ''),
		       COALESCE(custom_fields, '{}'),
		       updated_at
		FROM %s WHERE id = 1
	`, r.tableName)

	var cfg DBConfig
	err := r.db.QueryRow(query).Scan(
		&cfg.ID,
		&cfg.Enabled,
		&cfg.AsyncMode,
		&cfg.BufferSize,
		&cfg.SkipPaths,
		&cfg.CustomFields,
		&cfg.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// 如果不存在，插入默认配置
		defaultCfg := r.getDefaultConfig()
		if err := r.SaveConfig(defaultCfg); err != nil {
			return nil, err
		}
		return defaultCfg, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig 保存配置到数据库
func (r *ConfigRepository) SaveConfig(cfg *DBConfig) error {
	// 使用 DELETE + INSERT 确保数据正确更新
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	// 删除旧配置
	_, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = 1", r.tableName))
	if err != nil {
		tx.Rollback()
		return err
	}

	// 插入新配置
	query := fmt.Sprintf(`
		INSERT INTO %s (id, enabled, async_mode, buffer_size, skip_paths, custom_fields, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, NOW())
	`, r.tableName)

	_, err = tx.Exec(query,
		cfg.Enabled,
		cfg.AsyncMode,
		cfg.BufferSize,
		cfg.SkipPaths,
		cfg.CustomFields,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// ResetConfig 重置配置为默认值
func (r *ConfigRepository) ResetConfig() error {
	cfg := r.getDefaultConfig()
	return r.SaveConfig(cfg)
}

func (r *ConfigRepository) getDefaultConfig() *DBConfig {
	return &DBConfig{
		ID:           1,
		Enabled:      true,
		AsyncMode:    true,
		BufferSize:   1000,
		SkipPaths:    "/health,/metrics",
		CustomFields: "{}",
	}
}

// InitConfigTable 初始化配置表
func (r *ConfigRepository) InitConfigTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INT PRIMARY KEY DEFAULT 1,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			async_mode BOOLEAN NOT NULL DEFAULT TRUE,
			buffer_size INT NOT NULL DEFAULT 1000,
			skip_paths TEXT,
			custom_fields JSONB,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`, r.tableName)

	_, err := r.db.Exec(query)
	if err != nil {
		return err
	}

	// 确保默认配置存在
	cfg, err := r.LoadConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		return r.SaveConfig(r.getDefaultConfig())
	}
	return nil
}

// ParseSkipPaths 解析跳过路径
func ParseSkipPaths(paths string) []string {
	if paths == "" {
		return nil
	}
	var result []string
	for _, p := range splitComma(paths) {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// JoinSkipPaths 合并跳过路径
func JoinSkipPaths(paths []string) string {
	return strings.Join(paths, ",")
}

// ParseCustomFields 解析自定义字段
func ParseCustomFields(jsonStr string) map[string]interface{} {
	if jsonStr == "" || jsonStr == "{}" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	return result
}

// MarshalCustomFields 序列化自定义字段
func MarshalCustomFields(fields map[string]interface{}) string {
	if fields == nil {
		return "{}"
	}
	data, _ := json.Marshal(fields)
	return string(data)
}

func splitComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
