package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"monitor-agent/config"
	"monitor-agent/impact"
	"monitor-agent/monitor"
	"monitor-agent/provider"
	"monitor-agent/server"
	"monitor-agent/types"
)

// Config 服务配置
type Config struct {
	Addr       string
	LogDir     string
	ConfigFile string
}

// Service 监控服务
type Service struct {
	config     Config
	appConfig  *config.Config
	mm         *monitor.MultiMonitor
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
}

// New 创建服务实例（使用默认配置）
func New(cfg Config) (*Service, error) {
	return NewWithConfig(cfg, config.DefaultConfig())
}

// NewWithConfig 创建服务实例（使用指定配置）
func NewWithConfig(cfg Config, appCfg *config.Config) (*Service, error) {
	// 确保日志目录存在
	if cfg.LogDir == "" {
		exe, _ := os.Executable()
		cfg.LogDir = filepath.Join(filepath.Dir(exe), "logs")
	}
	os.MkdirAll(cfg.LogDir, 0755)

	// 设置日志输出
	if appCfg.Logging.FileOutput {
		logFile := filepath.Join(cfg.LogDir, "service.log")
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if appCfg.Logging.ConsoleOutput {
				// 同时输出到文件和控制台
				log.SetOutput(f)
			} else {
				log.SetOutput(f)
			}
		}
	}

	monitorCfg := types.MultiMonitorConfig{
		SampleInterval:   appCfg.Sampling.Interval,
		MetricsBufferLen: appCfg.Sampling.MetricsBufferLen,
		EventsBufferLen:  appCfg.Sampling.EventsBufferLen,
		LogDir:           cfg.LogDir,
	}

	prov := provider.New()
	mm, err := monitor.NewMultiMonitor(monitorCfg, prov)
	if err != nil {
		return nil, fmt.Errorf("create multi monitor: %w", err)
	}

	// 创建影响分析器
	if appCfg.Impact.Enabled {
		analyzer := impact.NewImpactAnalyzer(
			appCfg.Impact,
			prov,
			mm.GetTargets,
			mm.ListAllProcesses,
		)
		// 设置事件回调，将影响事件记录到事件日志
		analyzer.SetEventCallback(func(eventType string, pid int32, name string, message string) {
			mm.AddImpactEvent(eventType, pid, name, message)
		})
		mm.SetImpactAnalyzer(analyzer)
		log.Printf("[SERVICE] Impact analyzer enabled (interval=%ds)", appCfg.Impact.AnalysisInterval)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		config:    cfg,
		appConfig: appCfg,
		mm:        mm,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

// Start 启动服务
func (s *Service) Start() error {
	log.Printf("[SERVICE] Starting monitor service...")
	log.Printf("[SERVICE] Log directory: %s", s.config.LogDir)

	// 启动监控
	s.mm.Start()

	// 从配置文件加载监控目标
	if err := s.loadTargetsFromConfig(); err != nil {
		log.Printf("[SERVICE] Load targets from config failed: %v", err)
	}

	// 启动 HTTP 服务器（如果启用）
	if s.appConfig.Server.Enabled {
		webSrv := server.NewWebServerWithConfig(s.mm, server.AuthConfig{}, s.appConfig, s.config.ConfigFile)
		s.httpServer = &http.Server{
			Addr:    s.config.Addr,
			Handler: webSrv,
		}

		go func() {
			log.Printf("[SERVICE] HTTP server listening on %s", s.config.Addr)
			if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[SERVICE] HTTP server error: %v", err)
			}
		}()
	} else {
		log.Printf("[SERVICE] HTTP server disabled")
	}

	log.Printf("[SERVICE] Service started successfully")
	return nil
}

// Stop 停止服务
func (s *Service) Stop() error {
	log.Printf("[SERVICE] Stopping monitor service...")

	// 停止监控
	s.mm.Stop()

	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("[SERVICE] HTTP server shutdown error: %v", err)
		}
	}

	s.cancel()
	log.Printf("[SERVICE] Service stopped")
	return nil
}

// Wait 等待服务结束
func (s *Service) Wait() {
	<-s.ctx.Done()
}

// GetMonitor 获取监控器实例
func (s *Service) GetMonitor() *monitor.MultiMonitor {
	return s.mm
}

// loadTargetsFromConfig 从配置文件加载监控目标
func (s *Service) loadTargetsFromConfig() error {
	if len(s.appConfig.Targets) == 0 {
		log.Printf("[SERVICE] No targets in config")
		return nil
	}

	log.Printf("[SERVICE] Loading %d targets from config...", len(s.appConfig.Targets))

	// 获取当前进程列表
	processes, err := s.mm.ListAllProcesses()
	if err != nil {
		return fmt.Errorf("list processes: %w", err)
	}

	// 构建进程名到 PID 的映射
	nameToProcs := make(map[string][]types.ProcessInfo)
	for i := range processes {
		p := &processes[i]
		nameToProcs[p.Name] = append(nameToProcs[p.Name], *p)
	}

	// 添加监控目标
	for _, target := range s.appConfig.Targets {
		// 如果指定了 PID，直接使用
		if target.PID > 0 {
			if err := s.mm.AddTarget(target); err != nil {
				log.Printf("[SERVICE] Add target PID %d failed: %v", target.PID, err)
			} else {
				log.Printf("[SERVICE] Added target: %s (PID %d)", target.Name, target.PID)
			}
			continue
		}

		// 按进程名查找
		if target.Name == "" {
			log.Printf("[SERVICE] Skip target: no PID or name specified")
			continue
		}

		procs, found := nameToProcs[target.Name]
		if !found || len(procs) == 0 {
			log.Printf("[SERVICE] Process '%s' not found", target.Name)
			continue
		}

		if len(procs) > 1 {
			log.Printf("[SERVICE] Multiple processes found for '%s', using first one (PID %d)",
				target.Name, procs[0].PID)
		}

		// 使用找到的第一个进程
		target.PID = procs[0].PID
		target.Cmdline = procs[0].Cmdline
		if err := s.mm.AddTarget(target); err != nil {
			log.Printf("[SERVICE] Add target '%s' failed: %v", target.Name, err)
		} else {
			log.Printf("[SERVICE] Added target: %s (PID %d)", target.Name, target.PID)
		}
	}

	return nil
}
