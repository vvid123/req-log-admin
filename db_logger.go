package reqlogmid

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// DBLogger 数据库日志输出实现
type DBLogger struct {
	db        *sql.DB
	driver    string
	tableName string
	bufferCh  chan *LogEntry
	wg        sync.WaitGroup
	quit      chan struct{}
	closed    bool
	mu        sync.Mutex
}

// DBConfig 数据库连接配置
type DBConfig struct {
	Driver          string        // mysql, postgres, sqlite
	DSN             string        // 数据源名称
	TableName       string        // 表名，默认 "request_logs"
	MaxOpenConns    int           // 最大打开连接数
	MaxIdleConns    int           // 最大空闲连接数
	ConnMaxLifetime time.Duration // 连接最大生命周期
}

// NewDBLogger 创建数据库日志输出器
func NewDBLogger(driver, dsn string, async bool, bufferSize int) (*DBLogger, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 设置时区
	setTimezone(db, driver)

	logger := &DBLogger{
		db:        db,
		driver:    driver,
		tableName: "request_logs",
		bufferCh:  make(chan *LogEntry, bufferSize),
		quit:      make(chan struct{}),
	}

	if async {
		logger.startAsyncWriter()
	}

	return logger, nil
}

// NewDBLoggerWithConfig 使用配置创建数据库日志输出器
func NewDBLoggerWithConfig(cfg DBConfig, async bool, bufferSize int) (*DBLogger, error) {
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 设置时区
	setTimezone(db, cfg.Driver)

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "request_logs"
	}

	logger := &DBLogger{
		db:        db,
		driver:    cfg.Driver,
		tableName: tableName,
		bufferCh:  make(chan *LogEntry, bufferSize),
		quit:      make(chan struct{}),
	}

	if async {
		logger.startAsyncWriter()
	}

	return logger, nil
}

// SetTableName 设置表名
func (l *DBLogger) SetTableName(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tableName = name
}

// setTimezone 设置数据库时区为本地时间，确保读取时间正确
func setTimezone(db *sql.DB, driver string) {
	// 不设置数据库时区，让读取的时间保持原样（本地时间）
	// 这样数据库存储的本地时间会被正确读取
}

// convertToLocalTime 将 UTC 时间转换为本地时间
// 数据库存储的是本地时间，但被 sql 包当作 UTC 读取，这里转回本地时间
func convertToLocalTime(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	// 假设数据库存的是本地时间，但读取被当作 UTC
	// 需要手动转换为本地时区
	loc, _ := time.LoadLocation("Local")
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
}

// DB 获取数据库连接
func (l *DBLogger) DB() *sql.DB {
	return l.db
}

// startAsyncWriter 启动异步写入协程
func (l *DBLogger) startAsyncWriter() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			select {
			case entry, ok := <-l.bufferCh:
				if !ok {
					l.flushBuffer()
					return
				}
				if err := l.insertEntry(entry); err != nil {
					fmt.Fprintf(os.Stderr, "failed to insert log: %v\n", err)
				}
			case <-l.quit:
				l.flushBuffer()
				return
			}
		}
	}()
}

// insertEntry 插入单条日志
func (l *DBLogger) insertEntry(entry *LogEntry) error {
	customFields, _ := json.Marshal(entry.CustomFields)

	query := fmt.Sprintf(`
		INSERT INTO %s (method, path, client_ip, user_agent, status_code, duration_ms, timestamp, custom_fields, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, l.tableName)

	_, err := l.db.Exec(query,
		entry.Method,
		entry.Path,
		entry.ClientIP,
		entry.UserAgent,
		entry.StatusCode,
		entry.Duration,
		entry.Timestamp,
		customFields,
		time.Now(),
	)
	return err
}

// Write 实现 Logger 接口
func (l *DBLogger) Write(entry *LogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return fmt.Errorf("logger is closed")
	}

	if l.bufferCh != nil {
		select {
		case l.bufferCh <- entry:
			return nil
		default:
			// 缓冲区满，丢弃日志
			fmt.Fprintf(os.Stderr, "log buffer full, dropping entry: %s %s\n", entry.Method, entry.Path)
			return nil
		}
	}

	// 同步模式
	return l.insertEntry(entry)
}

// Close 实现 Logger 接口
func (l *DBLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	if l.bufferCh != nil {
		close(l.bufferCh)
		l.wg.Wait()
	}

	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

// Flush 实现 Logger 接口
func (l *DBLogger) Flush() {
	if l.bufferCh == nil {
		return
	}
	l.flushBuffer()
}

// flushBuffer 清空缓冲区中的所有日志
func (l *DBLogger) flushBuffer() {
	for {
		select {
		case entry := <-l.bufferCh:
			if err := l.insertEntry(entry); err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush log: %v\n", err)
			}
		default:
			return
		}
	}
}

// DBLogEntry 从数据库读取的日志条目
type DBLogEntry struct {
	ID           int64     `json:"id"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	ClientIP     string    `json:"client_ip"`
	UserAgent    string    `json:"user_agent"`
	StatusCode   int       `json:"status_code"`
	Duration     float64   `json:"duration_ms"`
	Timestamp    string    `json:"timestamp"`
	CustomFields string    `json:"custom_fields"`
	CreatedAt    time.Time `json:"created_at"`
}

// QueryLogs 查询日志
func (l *DBLogger) QueryLogs(offset, limit int, conditions map[string]interface{}) ([]DBLogEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, method, path, client_ip, user_agent, status_code, duration_ms, timestamp, custom_fields, created_at
		FROM %s
	`, l.tableName)

	args := []interface{}{}
	conds := []string{}
	argNum := 1

	if method, ok := conditions["method"]; ok && method != "" {
		conds = append(conds, fmt.Sprintf("method = $%d", argNum))
		args = append(args, method)
		argNum++
	}
	if path, ok := conditions["path"]; ok && path != "" {
		if pathStr, ok := path.(string); ok && pathStr != "" {
			conds = append(conds, fmt.Sprintf("path LIKE $%d", argNum))
			args = append(args, "%"+pathStr+"%")
			argNum++
		}
	}
	if statusCode, ok := conditions["status_code"]; ok && statusCode != 0 {
		conds = append(conds, fmt.Sprintf("status_code = $%d", argNum))
		args = append(args, statusCode)
		argNum++
	}
	if startTime, ok := conditions["start_time"]; ok && startTime != "" {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, startTime)
		argNum++
	}
	if endTime, ok := conditions["end_time"]; ok && endTime != "" {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", argNum))
		args = append(args, endTime)
		argNum++
	}

	if len(conds) > 0 {
		query += " WHERE " + joinStrings(conds, " AND ")
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, limit, offset)

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []DBLogEntry
	for rows.Next() {
		var entry DBLogEntry
		if err := rows.Scan(
			&entry.ID, &entry.Method, &entry.Path, &entry.ClientIP,
			&entry.UserAgent, &entry.StatusCode, &entry.Duration,
			&entry.Timestamp, &entry.CustomFields, &entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	// 转换时区：数据库存的是本地时间，但被当作 UTC 读取
	for i := range entries {
		entries[i].CreatedAt = convertToLocalTime(entries[i].CreatedAt)
	}

	return entries, nil
}

// CountLogs 统计日志数量
func (l *DBLogger) CountLogs(conditions map[string]interface{}) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", l.tableName)

	args := []interface{}{}
	conds := []string{}
	argNum := 1

	if method, ok := conditions["method"]; ok && method != "" {
		conds = append(conds, fmt.Sprintf("method = $%d", argNum))
		args = append(args, method)
		argNum++
	}
	if startTime, ok := conditions["start_time"]; ok && startTime != "" {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", argNum))
		args = append(args, startTime)
		argNum++
	}
	if endTime, ok := conditions["end_time"]; ok && endTime != "" {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", argNum))
		args = append(args, endTime)
		argNum++
	}

	if len(conds) > 0 {
		query += " WHERE " + joinStrings(conds, " AND ")
	}

	var count int64
	err := l.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// GetLogByID 根据ID获取单条日志
func (l *DBLogger) GetLogByID(id int64) (*DBLogEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, method, path, client_ip, user_agent, status_code, duration_ms, timestamp, custom_fields, created_at
		FROM %s WHERE id = $1
	`, l.tableName)

	var entry DBLogEntry
	err := l.db.QueryRow(query, id).Scan(
		&entry.ID, &entry.Method, &entry.Path, &entry.ClientIP,
		&entry.UserAgent, &entry.StatusCode, &entry.Duration,
		&entry.Timestamp, &entry.CustomFields, &entry.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// 转换时区
	entry.CreatedAt = convertToLocalTime(entry.CreatedAt)
	return &entry, nil
}

// DeleteOldLogs 删除指定天数之前的日志
func (l *DBLogger) DeleteOldLogs(days int) (int64, error) {
	var query string
	if l.isMySQL() {
		query = fmt.Sprintf("DELETE FROM %s WHERE created_at < DATE_SUB(NOW(), INTERVAL %d DAY)", l.tableName, days)
	} else {
		query = fmt.Sprintf("DELETE FROM %s WHERE created_at < NOW() - INTERVAL '%d days'", l.tableName, days)
	}
	result, err := l.db.Exec(query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// isMySQL 判断是否为 MySQL 数据库
func (l *DBLogger) isMySQL() bool {
	return l.driver == "mysql"
}

// GetTodayLogsCount 获取今日请求数
func (l *DBLogger) GetTodayLogsCount() (int64, error) {
	var query string
	if l.isMySQL() {
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE DATE(created_at) = CURDATE()", l.tableName)
	} else {
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE DATE(created_at) = CURRENT_DATE", l.tableName)
	}
	var count int64
	err := l.db.QueryRow(query).Scan(&count)
	return count, err
}

// GetTotalLogsCount 获取总请求数
func (l *DBLogger) GetTotalLogsCount() (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", l.tableName)
	var count int64
	err := l.db.QueryRow(query).Scan(&count)
	return count, err
}

// GetAvgDuration 获取平均响应时间（毫秒）
func (l *DBLogger) GetAvgDuration() (float64, error) {
	query := fmt.Sprintf("SELECT COALESCE(AVG(duration_ms), 0) FROM %s", l.tableName)
	var avg float64
	err := l.db.QueryRow(query).Scan(&avg)
	return avg, err
}

// GetErrorRate 获取错误率（百分比）
func (l *DBLogger) GetErrorRate() (float64, error) {
	// 错误码定义为 400 及以上
	query := fmt.Sprintf(`
		SELECT COALESCE(
			100.0 * SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0),
			0
		) FROM %s
	`, l.tableName)
	var rate float64
	err := l.db.QueryRow(query).Scan(&rate)
	return rate, err
}

// GetStats 获取统计数据
func (l *DBLogger) GetStats() (int64, int64, float64, float64, error) {
	todayCount, err1 := l.GetTodayLogsCount()
	totalCount, err2 := l.GetTotalLogsCount()
	avgDuration, err3 := l.GetAvgDuration()
	errorRate, err4 := l.GetErrorRate()

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return 0, 0, 0, 0, fmt.Errorf("统计查询失败: %v %v %v %v", err1, err2, err3, err4)
	}

	return todayCount, totalCount, avgDuration, errorRate, nil
}

// CreateTable 创建日志表（PostgreSQL 语法）
func (l *DBLogger) CreateTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			method VARCHAR(10) NOT NULL,
			path VARCHAR(512) NOT NULL,
			client_ip VARCHAR(45) NOT NULL,
			user_agent VARCHAR(512),
			status_code INT NOT NULL,
			duration_ms DOUBLE PRECISION NOT NULL,
			timestamp VARCHAR(32) NOT NULL,
			custom_fields JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`, l.tableName)

	_, err := l.db.Exec(query)
	if err != nil {
		return err
	}

	// 创建索引
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_method ON %s(method)", l.tableName, l.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_path ON %s(path)", l.tableName, l.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_status_code ON %s(status_code)", l.tableName, l.tableName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at)", l.tableName, l.tableName),
	}

	for _, idx := range indexes {
		l.db.Exec(idx)
	}

	return nil
}

// CreateTableSQL 返回建表 SQL（PostgreSQL）
func (l *DBLogger) CreateTableSQL() string {
	return fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id BIGSERIAL PRIMARY KEY,
			method VARCHAR(10) NOT NULL,
			path VARCHAR(512) NOT NULL,
			client_ip VARCHAR(45) NOT NULL,
			user_agent VARCHAR(512),
			status_code INT NOT NULL,
			duration_ms DOUBLE PRECISION NOT NULL,
			timestamp VARCHAR(32) NOT NULL,
			custom_fields JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_%s_method ON %s(method);
		CREATE INDEX IF NOT EXISTS idx_%s_path ON %s(path);
		CREATE INDEX IF NOT EXISTS idx_%s_status_code ON %s(status_code);
		CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s(created_at);
	`, l.tableName, l.tableName, l.tableName, l.tableName, l.tableName, l.tableName, l.tableName, l.tableName, l.tableName)
}

// joinStrings 辅助函数
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// EscapeLike 防止 SQL LIKE 注入
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
