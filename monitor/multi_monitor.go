package monitor

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"monitor-agent/buffer"
	"monitor-agent/impact"
	"monitor-agent/logger"
	"monitor-agent/provider"
	"monitor-agent/types"
)

// MultiMonitor 多进程监控器
type MultiMonitor struct {
	mu             sync.RWMutex
	provider       provider.ProcProvider
	targets        map[int32]*targetState // PID -> 状态
	metricsBuffers map[int32]*buffer.RingBuffer[types.ProcessMetrics]
	eventsBuffer   *buffer.RingBuffer[types.Event]
	config         types.MultiMonitorConfig
	running        bool
	stopCh         chan struct{}

	// 进程变化追踪
	processTracker *ProcessTracker

	// 影响分析器
	impactAnalyzer *impact.ImpactAnalyzer
}

type targetState struct {
	target       types.MonitorTarget
	lastMetric   *types.ProcessMetrics
	exitReported bool // 是否已报告退出事件
}

func NewMultiMonitor(cfg types.MultiMonitorConfig, prov provider.ProcProvider) (*MultiMonitor, error) {
	if cfg.SampleInterval <= 0 {
		cfg.SampleInterval = 1
	}
	if cfg.MetricsBufferLen <= 0 {
		cfg.MetricsBufferLen = 300 // 5分钟
	}
	if cfg.EventsBufferLen <= 0 {
		cfg.EventsBufferLen = 100
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}

	m := &MultiMonitor{
		provider:       prov,
		targets:        make(map[int32]*targetState),
		metricsBuffers: make(map[int32]*buffer.RingBuffer[types.ProcessMetrics]),
		eventsBuffer:   buffer.NewRingBuffer[types.Event](cfg.EventsBufferLen),
		config:         cfg,
		stopCh:         make(chan struct{}),
		processTracker: NewProcessTracker(200), // 保留最近 200 条进程变化
	}

	return m, nil
}

// SetImpactAnalyzer 设置影响分析器
func (m *MultiMonitor) SetImpactAnalyzer(analyzer *impact.ImpactAnalyzer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.impactAnalyzer = analyzer
}

// GetImpactAnalyzer 获取影响分析器
func (m *MultiMonitor) GetImpactAnalyzer() *impact.ImpactAnalyzer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.impactAnalyzer
}

// AddTarget 添加监控目标
func (m *MultiMonitor) AddTarget(target types.MonitorTarget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[target.PID]; exists {
		return fmt.Errorf("target PID %d already monitored", target.PID)
	}

	// 验证进程存在
	if !m.provider.IsAlive(target.PID) {
		return fmt.Errorf("process PID %d not found", target.PID)
	}

	// 立即获取一次指标
	var initialMetric *types.ProcessMetrics
	if met, err := m.provider.GetMetrics(target.PID); err == nil {
		met.Timestamp = time.Now()
		met.Alive = true
		initialMetric = met
	}

	state := &targetState{target: target, lastMetric: initialMetric}
	m.targets[target.PID] = state

	buf := buffer.NewRingBuffer[types.ProcessMetrics](m.config.MetricsBufferLen)
	if initialMetric != nil {
		buf.Push(*initialMetric)
	}
	m.metricsBuffers[target.PID] = buf

	logger.Infof("MONITOR", "Added monitor target: PID=%d Name=%s", target.PID, target.Name)
	return nil
}

// RemoveTarget 移除监控目标
func (m *MultiMonitor) RemoveTarget(pid int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.targets, pid)
	delete(m.metricsBuffers, pid)

	// 清理该目标的影响事件
	if m.impactAnalyzer != nil {
		m.impactAnalyzer.RemoveTargetEvents(pid)
	}

	logger.Infof("MONITOR", "Removed monitor target: PID=%d", pid)
}

// RemoveAllTargets 移除所有监控目标
func (m *MultiMonitor) RemoveAllTargets() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targets = make(map[int32]*targetState)
	m.metricsBuffers = make(map[int32]*buffer.RingBuffer[types.ProcessMetrics])

	// 清理所有影响事件
	if m.impactAnalyzer != nil {
		m.impactAnalyzer.ClearAllEvents()
	}

	logger.Info("MONITOR", "Removed all monitor targets")
}

// UpdateTarget 更新监控目标配置
func (m *MultiMonitor) UpdateTarget(target types.MonitorTarget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.targets[target.PID]
	if !exists {
		return fmt.Errorf("target PID %d not found", target.PID)
	}

	state.target = target
	logger.Infof("MONITOR", "Updated monitor target: PID=%d Name=%s", target.PID, target.Name)
	return nil
}

// GetTargets 获取所有监控目标（按 PID 排序）
func (m *MultiMonitor) GetTargets() []types.MonitorTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 收集所有 PID 并排序
	pids := make([]int32, 0, len(m.targets))
	for pid := range m.targets {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })

	// 按排序后的顺序返回
	result := make([]types.MonitorTarget, 0, len(pids))
	for _, pid := range pids {
		result = append(result, m.targets[pid].target)
	}
	return result
}

// Start 启动监控
func (m *MultiMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.loop()
	logger.Info("MONITOR", "MultiMonitor started")

	// 启动影响分析器
	if m.impactAnalyzer != nil {
		m.impactAnalyzer.Start()
	}
}

// Stop 停止监控
func (m *MultiMonitor) Stop() {
	// 停止影响分析器
	if m.impactAnalyzer != nil {
		m.impactAnalyzer.Stop()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	close(m.stopCh)
	m.stopCh = make(chan struct{}) // 重新创建 channel 以便下次启动
	logger.Info("MONITOR", "MultiMonitor stopped")
}

func (m *MultiMonitor) loop() {
	ticker := time.NewTicker(time.Duration(m.config.SampleInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collectAll()
		}
	}
}

func (m *MultiMonitor) collectAll() {
	m.mu.Lock()
	pids := make([]int32, 0, len(m.targets))
	for pid := range m.targets {
		pids = append(pids, pid)
	}
	m.mu.Unlock()

	for _, pid := range pids {
		m.collectOne(pid)
	}
}

func (m *MultiMonitor) collectOne(pid int32) {
	m.mu.Lock()
	state, exists := m.targets[pid]
	if !exists {
		m.mu.Unlock()
		return
	}
	buf := m.metricsBuffers[pid]
	target := state.target
	m.mu.Unlock()

	alive := m.provider.IsAlive(pid)
	metric := types.ProcessMetrics{
		Timestamp: time.Now(),
		PID:       pid,
		Alive:     alive,
	}

	if alive {
		if met, err := m.provider.GetMetrics(pid); err == nil {
			metric = *met
			metric.Timestamp = time.Now()
			metric.Alive = true
		}
		// 进程恢复运行，重置退出标记
		m.mu.Lock()
		state.exitReported = false
		m.mu.Unlock()
	}

	buf.Push(metric)
	m.mu.Lock()
	state.lastMetric = &metric
	exitReported := state.exitReported
	m.mu.Unlock()

	// 写入日志
	logger.Metric(metric)

	// 检测进程退出事件
	if !alive && !exitReported {
		m.mu.Lock()
		state.exitReported = true
		m.mu.Unlock()

		evt := types.Event{
			Timestamp: time.Now(),
			Type:      "exit",
			PID:       pid,
			Name:      target.Name,
			Message:   "进程已退出",
		}
		m.addEvent(evt)
	}
}

func (m *MultiMonitor) addEvent(evt types.Event) {
	m.eventsBuffer.Push(evt)
	logger.Event(evt.Type, evt.PID, evt.Name, evt.Message)
}

// AddImpactEvent 添加影响事件到事件日志
func (m *MultiMonitor) AddImpactEvent(eventType string, pid int32, name string, message string) {
	evt := types.Event{
		Timestamp: time.Now(),
		Type:      eventType,
		PID:       pid,
		Name:      name,
		Message:   message,
	}
	m.addEvent(evt)
}

// GetMetrics 获取指定进程的最近指标
func (m *MultiMonitor) GetMetrics(pid int32, n int) []types.ProcessMetrics {
	m.mu.RLock()
	buf, exists := m.metricsBuffers[pid]
	m.mu.RUnlock()
	if !exists {
		return nil
	}
	return buf.GetRecent(n)
}

// GetAllLatestMetrics 获取所有监控目标的最新指标
func (m *MultiMonitor) GetAllLatestMetrics() map[int32]*types.ProcessMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[int32]*types.ProcessMetrics)
	for pid, state := range m.targets {
		if state.lastMetric != nil {
			result[pid] = state.lastMetric
		}
	}
	return result
}

// GetRecentEvents 获取最近事件
func (m *MultiMonitor) GetRecentEvents(n int) []types.Event {
	return m.eventsBuffer.GetRecent(n)
}

// IsRunning 检查是否运行中
func (m *MultiMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// ListAllProcesses 列出系统所有进程
func (m *MultiMonitor) ListAllProcesses() ([]types.ProcessInfo, error) {
	processes, err := m.provider.ListAllProcesses()
	if err != nil {
		return nil, err
	}

	// 更新进程追踪器
	changes := m.processTracker.Update(processes)

	// 将进程变化转换为事件
	for _, change := range changes {
		eventType := "new_process"
		message := "新进程启动"
		if change.Type == "gone" {
			eventType = "process_gone"
			message = "进程消失"
		}
		evt := types.Event{
			Timestamp: change.Timestamp,
			Type:      eventType,
			PID:       change.PID,
			Name:      change.Name,
			Message:   message,
		}
		m.addEvent(evt)
	}

	return processes, nil
}

// GetProcessChanges 获取最近的进程变化
func (m *MultiMonitor) GetProcessChanges(n int) []types.ProcessChange {
	return m.processTracker.GetRecentChanges(n)
}

// GetSystemMetrics 获取系统指标
func (m *MultiMonitor) GetSystemMetrics() (*types.SystemMetrics, error) {
	return m.provider.GetSystemMetrics()
}

// GetRecentImpacts 获取最近的影响事件
func (m *MultiMonitor) GetRecentImpacts(n int) []types.ImpactEvent {
	if m.impactAnalyzer == nil {
		return []types.ImpactEvent{}
	}
	return m.impactAnalyzer.GetRecentImpacts(n)
}

// GetImpactSummary 获取影响统计摘要
func (m *MultiMonitor) GetImpactSummary() map[string]interface{} {
	if m.impactAnalyzer == nil {
		return map[string]interface{}{"total": 0}
	}
	return m.impactAnalyzer.GetImpactSummary()
}
