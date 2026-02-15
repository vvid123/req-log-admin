package admin

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // PostgreSQL 驱动

	"github.com/zxyao/req-log-mid"
	"github.com/zxyao/req-log-mid/config"
)

//go:embed index.html
var adminHTML embed.FS

// StartOptions 启动选项
type StartOptions struct {
	ConfigPath string // 配置文件路径，默认 "config.yaml"
	Port       int    // 服务端口，默认 8080
}

// getCurrentDir 获取当前可执行文件所在目录
func getCurrentDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exePath)
}

// findConfigFile 查找配置文件
func findConfigFile(configPath string) string {
	exeDir := getCurrentDir()

	if filepath.IsAbs(configPath) {
		return configPath
	}

	paths := []string{
		configPath,
		filepath.Join(exeDir, configPath),
		filepath.Join(".", configPath),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return configPath
}

// NewLogger 创建数据库日志记录器（不启动服务器）
// 可用于在主应用中获取日志记录器并添加到主路由器
func NewLogger(configPath string) (*reqlogmid.DBLogger, error) {
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfgPath := findConfigFile(configPath)

	dbConfig, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	logger, err := reqlogmid.NewDBLogger(
		dbConfig.Database.Driver,
		dbConfig.Database.BuildDSN(),
		true,
		dbConfig.Database.MaxOpenConns,
	)
	if err != nil {
		return nil, fmt.Errorf("创建日志器失败: %w", err)
	}

	// 创建日志表
	if err := logger.CreateTable(); err != nil {
		log.Printf("创建日志表失败: %v", err)
	}

	return logger, nil
}

// Start 启动日志中间件服务
func Start(opts StartOptions) error {
	if opts.ConfigPath == "" {
		opts.ConfigPath = "config.yaml"
	}
	if opts.Port == 0 {
		opts.Port = 8080
	}

	exeDir := getCurrentDir()
	configPath := findConfigFile(opts.ConfigPath)

	fmt.Printf("程序目录: %s\n", exeDir)
	fmt.Printf("配置路径: %s\n", configPath)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("警告: 配置文件不存在，使用内置默认配置\n")
	}

	dbConfig, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		dbConfig = &config.Config{
			Database: config.DatabaseConfig{
				Driver:       "postgres",
				Host:         "8.138.107.192",
				Port:         15432,
				Username:     "vivid",
				Password:     "vivid",
				Name:         "plugins",
				SSLMode:      "disable",
				MaxOpenConns: 25,
				MaxIdleConns: 5,
			},
		}
		fmt.Printf("使用内置默认数据库配置\n")
	}

	fmt.Printf("数据库: %s:%d/%s\n", dbConfig.Database.Host, dbConfig.Database.Port, dbConfig.Database.Name)

	logger, err := reqlogmid.NewDBLogger(
		dbConfig.Database.Driver,
		dbConfig.Database.BuildDSN(),
		true,
		dbConfig.Database.MaxOpenConns,
	)
	if err != nil {
		fmt.Printf("创建日志器失败: %v\n", err)
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	defer logger.Close()

	fmt.Printf("日志器创建成功，开始初始化...\n")

	if err := logger.CreateTable(); err != nil {
		log.Printf("创建日志表失败: %v", err)
	}

	configRepo := NewConfigRepository(logger.DB())
	if err := configRepo.InitConfigTable(); err != nil {
		log.Printf("初始化配置表失败: %v", err)
	}

	dbCfg, err := configRepo.LoadConfig()
	if err != nil {
		log.Printf("加载配置失败，使用默认配置: %v", err)
		dbCfg = &DBConfig{Enabled: true, AsyncMode: true, BufferSize: 1000}
	}

	logConfig := reqlogmid.DefaultConfig()
	logConfig.Enabled = dbCfg.Enabled
	logConfig.Async = dbCfg.AsyncMode
	logConfig.BufferSize = dbCfg.BufferSize
	logConfig.SkipPaths = ParseSkipPaths(dbCfg.SkipPaths)
	logConfig.CustomFields = ParseCustomFields(dbCfg.CustomFields)

	r := gin.Default()

	r.Use(reqlogmid.RequestLoggerWithConfig(logger, logConfig))

	logHandler := NewLogAdminHandler(logger)
	configHandler := NewConfigAdminHandler(configRepo, logConfig)
	RegisterRoutes(r, logHandler, configHandler)

	// 使用嵌入的静态文件
	r.GET("/admin", func(c *gin.Context) {
		data, err := adminHTML.ReadFile("index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Admin page not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Flush()
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", opts.Port)
	fmt.Printf("\n====================================\n")
	fmt.Printf("服务启动: http://localhost%s\n", addr)
	fmt.Printf("管理界面: http://localhost%s/admin\n", addr)
	fmt.Printf("====================================\n\n")

	return r.Run(addr)
}
