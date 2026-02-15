package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxyao/req-log-mid"
)

// LogAdminHandler 日志管理处理器
type LogAdminHandler struct {
	logger *reqlogmid.DBLogger
}

// NewLogAdminHandler 创建日志管理处理器
func NewLogAdminHandler(logger *reqlogmid.DBLogger) *LogAdminHandler {
	return &LogAdminHandler{logger: logger}
}

// LogQueryParams 日志查询参数
type LogQueryParams struct {
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
}

// LogListResponse 日志列表响应
type LogListResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Total    int64                  `json:"total"`
		Page     int                    `json:"page"`
		PageSize int                    `json:"page_size"`
		Logs     []reqlogmid.DBLogEntry `json:"logs"`
	} `json:"data"`
}

// GetLogs 获取日志列表
// @Summary 获取请求日志列表
// @Description 分页查询请求日志
// @Tags 日志管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param method query string false "HTTP方法"
// @Param path query string false "路径模糊搜索"
// @Param status_code query int false "状态码"
// @Param start_time query string false "开始时间"
// @Param end_time query string false "结束时间"
// @Success 200 {object} LogListResponse
// @Router /admin/logs [get]
func (h *LogAdminHandler) GetLogs(c *gin.Context) {
	// 解析查询参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	params := LogQueryParams{
		Page:       page,
		PageSize:   pageSize,
		Method:     c.Query("method"),
		Path:       c.Query("path"),
		StatusCode: -1,
		StartTime:  c.Query("start_time"),
		EndTime:    c.Query("end_time"),
	}
	if sc, err := strconv.Atoi(c.Query("status_code")); err == nil {
		params.StatusCode = sc
	}

	// 构建查询条件
	conditions := map[string]interface{}{}
	if params.Method != "" {
		conditions["method"] = params.Method
	}
	if params.StatusCode > 0 {
		conditions["status_code"] = params.StatusCode
	}
	if params.StartTime != "" {
		conditions["start_time"] = params.StartTime
	}
	if params.EndTime != "" {
		conditions["end_time"] = params.EndTime
	}
	// path 使用 LIKE 查询，在 DBLogger 中处理

	// 查询总数
	total, err := h.logger.CountLogs(conditions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询总数失败: " + err.Error(),
		})
		return
	}

	// 查询数据
	offset := (page - 1) * pageSize
	logs, err := h.logger.QueryLogs(offset, pageSize, conditions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询日志失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"logs":      logs,
		},
	})
}

// GetLogDetail 获取单条日志详情
func (h *LogAdminHandler) GetLogDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的日志ID",
		})
		return
	}

	log, err := h.logger.GetLogByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "查询日志失败: " + err.Error(),
		})
		return
	}

	if log == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "日志不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    log,
	})
}

// DeleteLogs 删除日志
func (h *LogAdminHandler) DeleteLogs(c *gin.Context) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || days < 1 {
		days = 7
	}

	count, err := h.logger.DeleteOldLogs(days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除日志失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"deleted_count": count,
		},
	})
}

// GetStats 获取统计数据
func (h *LogAdminHandler) GetStats(c *gin.Context) {
	// 从数据库获取统计信息
	todayLogs, totalLogs, avgDuration, errorRate, err := h.logger.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取统计数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"today_logs":   todayLogs,
			"total_logs":   totalLogs,
			"avg_duration": avgDuration,
			"error_rate":   errorRate,
		},
	})
}

// ConfigAdminHandler 配置管理处理器
type ConfigAdminHandler struct {
	repo   *ConfigRepository
	config *reqlogmid.Config
}

// NewConfigAdminHandler 创建配置管理处理器
func NewConfigAdminHandler(repo *ConfigRepository, cfg *reqlogmid.Config) *ConfigAdminHandler {
	return &ConfigAdminHandler{
		repo:   repo,
		config: cfg,
	}
}

// GetConfig 获取当前配置
func (h *ConfigAdminHandler) GetConfig(c *gin.Context) {
	// 从数据库加载配置
	cfg, err := h.repo.LoadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "加载配置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"enabled":       cfg.Enabled,
			"skip_paths":    ParseSkipPaths(cfg.SkipPaths),
			"custom_fields": ParseCustomFields(cfg.CustomFields),
			"async":         cfg.AsyncMode,
			"buffer_size":   cfg.BufferSize,
		},
	})
}

// UpdateConfigRequest 更新配置请求
type UpdateConfigRequest struct {
	Enabled      *bool                  `json:"enabled"`
	SkipPaths    []string               `json:"skip_paths"`
	CustomFields map[string]interface{} `json:"custom_fields"`
	Async        *bool                  `json:"async"`
	BufferSize   *int                   `json:"buffer_size"`
}

// UpdateConfig 更新配置
func (h *ConfigAdminHandler) UpdateConfig(c *gin.Context) {
	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 先加载当前配置
	cfg, err := h.repo.LoadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "加载配置失败: " + err.Error(),
		})
		return
	}

	// 更新配置
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.SkipPaths != nil {
		cfg.SkipPaths = JoinSkipPaths(req.SkipPaths)
	}
	if req.CustomFields != nil {
		cfg.CustomFields = MarshalCustomFields(req.CustomFields)
	}
	if req.Async != nil {
		cfg.AsyncMode = *req.Async
	}
	if req.BufferSize != nil {
		cfg.BufferSize = *req.BufferSize
	}

	// 保存到数据库
	if err := h.repo.SaveConfig(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "保存配置失败: " + err.Error(),
		})
		return
	}

	// 更新内存中的配置（使用写锁）
	h.config.Lock()
	h.config.Enabled = cfg.Enabled
	h.config.SkipPaths = ParseSkipPaths(cfg.SkipPaths)
	h.config.CustomFields = ParseCustomFields(cfg.CustomFields)
	h.config.Async = cfg.AsyncMode
	h.config.BufferSize = cfg.BufferSize
	h.config.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "配置更新成功",
	})
}

// ResetConfig 重置为默认配置
func (h *ConfigAdminHandler) ResetConfig(c *gin.Context) {
	// 重置数据库中的配置
	if err := h.repo.ResetConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "重置配置失败: " + err.Error(),
		})
		return
	}

	// 重置内存中的配置（使用写锁）
	defaultCfg := reqlogmid.DefaultConfig()
	h.config.Lock()
	h.config.Enabled = defaultCfg.Enabled
	h.config.SkipPaths = defaultCfg.SkipPaths
	h.config.CustomFields = defaultCfg.CustomFields
	h.config.TimeFormat = defaultCfg.TimeFormat
	h.config.Async = defaultCfg.Async
	h.config.BufferSize = defaultCfg.BufferSize
	h.config.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "配置已重置为默认值",
	})
}

// HealthCheck 健康检查
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// RegisterRoutes 注册管理路由
func RegisterRoutes(r *gin.Engine, logHandler *LogAdminHandler, configHandler *ConfigAdminHandler) {
	// 健康检查
	r.GET("/admin/health", HealthCheck)

	// 日志管理
	admin := r.Group("/admin")
	{
		// 日志相关
		logs := admin.Group("/logs")
		{
			logs.GET("", logHandler.GetLogs)
			logs.GET("/:id", logHandler.GetLogDetail)
			logs.DELETE("", logHandler.DeleteLogs)
		}

		// 配置相关
		config := admin.Group("/config")
		{
			config.GET("", configHandler.GetConfig)
			config.PUT("", configHandler.UpdateConfig)
			config.POST("/reset", configHandler.ResetConfig)
		}

		// 统计
		admin.GET("/stats", logHandler.GetStats)
	}
}
