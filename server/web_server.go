package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"sync"

	"monitor-agent/config"
	"monitor-agent/monitor"
	"monitor-agent/types"
)

//go:embed static/*
var staticFiles embed.FS

// WebServer Web 服务器（带界面）
type WebServer struct {
	multiMonitor *monitor.MultiMonitor
	authManager  *AuthManager
	mux          *http.ServeMux
	handler      http.Handler
	
	// 配置管理
	configMu     sync.RWMutex
	appConfig    *config.Config
	configFile   string
}

func NewWebServer(mm *monitor.MultiMonitor) *WebServer {
	return NewWebServerWithAuth(mm, AuthConfig{}, nil, "")
}

func NewWebServerWithConfig(mm *monitor.MultiMonitor, authCfg AuthConfig, appCfg *config.Config, configFile string) *WebServer {
	return NewWebServerWithAuth(mm, authCfg, appCfg, configFile)
}

func NewWebServerWithAuth(mm *monitor.MultiMonitor, authCfg AuthConfig, appCfg *config.Config, configFile string) *WebServer {
	s := &WebServer{
		multiMonitor: mm,
		authManager:  NewAuthManager(authCfg),
		mux:          http.NewServeMux(),
		appConfig:    appCfg,
		configFile:   configFile,
	}

	// 登录相关路由（不需要认证）
	s.mux.HandleFunc("/login", s.authManager.HandleLogin)
	s.mux.HandleFunc("/api/login", s.authManager.HandleLogin)
	s.mux.HandleFunc("/api/logout", s.authManager.HandleLogout)

	// API 路由
	s.mux.HandleFunc("/api/processes", s.handleListProcesses)
	s.mux.HandleFunc("/api/monitor/targets", s.handleTargets)
	s.mux.HandleFunc("/api/monitor/add", s.handleAddTarget)
	s.mux.HandleFunc("/api/monitor/remove", s.handleRemoveTarget)
	s.mux.HandleFunc("/api/monitor/removeAll", s.handleRemoveAllTargets)
	s.mux.HandleFunc("/api/monitor/update", s.handleUpdateTarget)
	s.mux.HandleFunc("/api/monitor/start", s.handleStart)
	s.mux.HandleFunc("/api/monitor/stop", s.handleStop)
	s.mux.HandleFunc("/api/metrics", s.handleMetrics)
	s.mux.HandleFunc("/api/metrics/latest", s.handleLatestMetrics)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/process-changes", s.handleProcessChanges)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/system", s.handleSystem)
	s.mux.HandleFunc("/api/impacts", s.handleImpacts)
	s.mux.HandleFunc("/api/impacts/summary", s.handleImpactsSummary)
	s.mux.HandleFunc("/api/impacts/clear", s.handleImpactsClear)
	s.mux.HandleFunc("/api/config/impact", s.handleImpactConfig)

	// 静态文件
	staticFS, _ := fs.Sub(staticFiles, "static")
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// 应用认证中间件
	s.handler = s.authManager.AuthMiddleware(s.mux)

	return s
}

func (s *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		return
	}
	s.handler.ServeHTTP(w, r)
}

func (s *WebServer) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *WebServer) errorResponse(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /api/processes - 列出系统所有进程
func (s *WebServer) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	procs, err := s.multiMonitor.ListAllProcesses()
	if err != nil {
		s.errorResponse(w, 500, err.Error())
		return
	}
	s.jsonResponse(w, procs)
}

// GET /api/monitor/targets - 获取监控目标列表
func (s *WebServer) handleTargets(w http.ResponseWriter, r *http.Request) {
	targets := s.multiMonitor.GetTargets()
	if targets == nil {
		targets = []types.MonitorTarget{}
	}
	s.jsonResponse(w, targets)
}

// POST /api/monitor/add - 添加监控目标
func (s *WebServer) handleAddTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	var target types.MonitorTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		s.errorResponse(w, 400, "invalid request body")
		return
	}
	if err := s.multiMonitor.AddTarget(target); err != nil {
		s.errorResponse(w, 400, err.Error())
		return
	}
	// 添加后自动启动监控
	s.multiMonitor.Start()
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// POST /api/monitor/remove - 移除监控目标
func (s *WebServer) handleRemoveTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	var req struct {
		PID int32 `json:"pid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.errorResponse(w, 400, "invalid request body")
		return
	}
	s.multiMonitor.RemoveTarget(req.PID)
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// POST /api/monitor/removeAll - 移除所有监控目标
func (s *WebServer) handleRemoveAllTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	s.multiMonitor.RemoveAllTargets()
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// POST /api/monitor/update - 更新监控目标配置
func (s *WebServer) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	var target types.MonitorTarget
	if err := json.NewDecoder(r.Body).Decode(&target); err != nil {
		s.errorResponse(w, 400, "invalid request body")
		return
	}
	if err := s.multiMonitor.UpdateTarget(target); err != nil {
		s.errorResponse(w, 400, err.Error())
		return
	}
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// POST /api/monitor/start - 启动监控
func (s *WebServer) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	s.multiMonitor.Start()
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// POST /api/monitor/stop - 停止监控
func (s *WebServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	s.multiMonitor.Stop()
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// GET /api/metrics?pid=xxx&n=100 - 获取指定进程的历史指标
func (s *WebServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	pidStr := r.URL.Query().Get("pid")
	pid, _ := strconv.ParseInt(pidStr, 10, 32)
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 60
	}
	metrics := s.multiMonitor.GetMetrics(int32(pid), n)
	if metrics == nil {
		metrics = []types.ProcessMetrics{}
	}
	s.jsonResponse(w, metrics)
}

// GET /api/metrics/latest - 获取所有监控目标的最新指标
func (s *WebServer) handleLatestMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.multiMonitor.GetAllLatestMetrics()
	s.jsonResponse(w, metrics)
}

// GET /api/events?n=50 - 获取最近事件
func (s *WebServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 50
	}
	events := s.multiMonitor.GetRecentEvents(n)
	if events == nil {
		events = []types.Event{}
	}
	s.jsonResponse(w, events)
}

// GET /api/process-changes?n=50 - 获取最近进程变化
func (s *WebServer) handleProcessChanges(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 50
	}
	changes := s.multiMonitor.GetProcessChanges(n)
	if changes == nil {
		changes = []types.ProcessChange{}
	}
	s.jsonResponse(w, changes)
}

// GET /api/status - 获取监控状态
func (s *WebServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]any{
		"running": s.multiMonitor.IsRunning(),
		"targets": len(s.multiMonitor.GetTargets()),
	})
}

// GET /api/system - 获取系统指标
func (s *WebServer) handleSystem(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.multiMonitor.GetSystemMetrics()
	if err != nil {
		s.errorResponse(w, 500, err.Error())
		return
	}
	s.jsonResponse(w, metrics)
}

// GET /api/impacts?n=50 - 获取最近影响事件
func (s *WebServer) handleImpacts(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	if n <= 0 {
		n = 50
	}
	impacts := s.multiMonitor.GetRecentImpacts(n)
	if impacts == nil {
		impacts = []types.ImpactEvent{}
	}
	s.jsonResponse(w, impacts)
}

// GET /api/impacts/summary - 获取影响统计摘要
func (s *WebServer) handleImpactsSummary(w http.ResponseWriter, r *http.Request) {
	summary := s.multiMonitor.GetImpactSummary()
	s.jsonResponse(w, summary)
}

// POST /api/impacts/clear - 清除所有影响事件
func (s *WebServer) handleImpactsClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.errorResponse(w, 405, "method not allowed")
		return
	}
	s.multiMonitor.ClearImpactEvents()
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// GET/POST /api/config/impact - 获取或更新影响分析配置
func (s *WebServer) handleImpactConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.configMu.RLock()
		defer s.configMu.RUnlock()
		
		if s.appConfig == nil {
			s.jsonResponse(w, config.DefaultConfig().Impact)
			return
		}
		s.jsonResponse(w, s.appConfig.Impact)
		return
	}
	
	if r.Method == "POST" {
		// 先读取当前配置作为基础
		s.configMu.Lock()
		defer s.configMu.Unlock()
		
		if s.appConfig == nil {
			s.appConfig = config.DefaultConfig()
		}
		
		// 解码到当前配置上（只覆盖 JSON 中存在的字段）
		if err := json.NewDecoder(r.Body).Decode(&s.appConfig.Impact); err != nil {
			s.errorResponse(w, 400, "invalid request body: "+err.Error())
			return
		}
		
		// 保存到文件
		if s.configFile != "" {
			if err := config.SaveConfig(s.configFile, s.appConfig); err != nil {
				s.errorResponse(w, 500, "save config failed: "+err.Error())
				return
			}
		}
		
		// 更新影响分析器配置
		analyzer := s.multiMonitor.GetImpactAnalyzer()
		if analyzer != nil {
			analyzer.UpdateConfig(s.appConfig.Impact)
		}
		
		s.jsonResponse(w, map[string]string{"status": "ok"})
		return
	}
	
	s.errorResponse(w, 405, "method not allowed")
}
