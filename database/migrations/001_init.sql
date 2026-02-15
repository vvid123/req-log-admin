-- =====================================================
-- Request Log Middleware 数据库迁移脚本
-- =====================================================

-- -----------------------------------------------------
-- request_logs - 请求日志表
-- -----------------------------------------------------
CREATE TABLE IF NOT EXISTS request_logs (
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

CREATE INDEX IF NOT EXISTS idx_request_logs_method ON request_logs(method);
CREATE INDEX IF NOT EXISTS idx_request_logs_path ON request_logs(path);
CREATE INDEX IF NOT EXISTS idx_request_logs_status_code ON request_logs(status_code);
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);

-- -----------------------------------------------------
-- log_config - 配置表
-- -----------------------------------------------------
CREATE TABLE IF NOT EXISTS log_config (
    id INT PRIMARY KEY DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    async_mode BOOLEAN NOT NULL DEFAULT TRUE,
    buffer_size INT NOT NULL DEFAULT 1000,
    skip_paths TEXT,
    custom_fields JSONB,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 插入默认配置
INSERT INTO log_config (id, enabled, async_mode, buffer_size, skip_paths)
VALUES (1, TRUE, TRUE, 1000, '/health,/metrics')
ON CONFLICT (id) DO NOTHING;
