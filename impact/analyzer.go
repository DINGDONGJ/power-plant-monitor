package impact

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"monitor-agent/logger"
	"monitor-agent/provider"
	"monitor-agent/types"
)

// impactKey 用于唯一标识一个影响事件
type impactKey struct {
	TargetPID  int32
	ImpactType string
	SourcePID  int32
	Detail     string // 对于文件/端口冲突，存储具体路径或端口
}

// EventCallback 事件回调函数类型
type EventCallback func(eventType string, pid int32, name string, message string)

// ImpactAnalyzer 影响分析器
type ImpactAnalyzer struct {
	mu           sync.RWMutex
	provider     provider.ProcProvider
	config       types.ImpactConfig
	targets      func() []types.MonitorTarget // 获取监控目标的函数
	getProcesses func() ([]types.ProcessInfo, error)
	running      bool
	stopCh       chan struct{}

	// 动态事件存储（活跃的冲突）
	activeImpacts map[impactKey]*types.ImpactEvent

	// 事件回调（用于记录到事件日志）
	eventCallback EventCallback

	// 文件和端口检测器
	fileChecker *FileChecker
	portChecker *PortChecker

	// 上次检测时间
	lastFileCheck time.Time
	lastPortCheck time.Time

	// 缓存监控目标的监听端口 (PID -> []port)
	targetPorts     map[int32][]int
	targetPortsTime time.Time

	// 缓存监控目标打开的文件 (PID -> []filePath)
	targetFiles     map[int32][]string
	targetFilesTime time.Time
}

// NewImpactAnalyzer 创建影响分析器
func NewImpactAnalyzer(
	cfg types.ImpactConfig,
	prov provider.ProcProvider,
	getTargets func() []types.MonitorTarget,
	getProcesses func() ([]types.ProcessInfo, error),
) *ImpactAnalyzer {
	// 设置必须有值的字段默认值（这些字段不能为0）
	if cfg.AnalysisInterval <= 0 {
		cfg.AnalysisInterval = 5
	}
	if cfg.TopNProcesses <= 0 {
		cfg.TopNProcesses = 10
	}
	if cfg.HistoryLen <= 0 {
		cfg.HistoryLen = 100
	}
	if cfg.FileCheckInterval <= 0 {
		cfg.FileCheckInterval = 30
	}
	if cfg.PortCheckInterval <= 0 {
		cfg.PortCheckInterval = 30
	}
	
	// 系统级别阈值默认值（这些也必须有值）
	if cfg.CPUThreshold <= 0 {
		cfg.CPUThreshold = 80
	}
	if cfg.MemoryThreshold <= 0 {
		cfg.MemoryThreshold = 85
	}
	if cfg.DiskIOThreshold <= 0 {
		cfg.DiskIOThreshold = 100
	}
	if cfg.NetworkThreshold <= 0 {
		cfg.NetworkThreshold = 100
	}
	
	// 进程级别阈值：不再覆盖！
	// 这些值应该从配置文件加载，0表示禁用检测
	// 配置文件的默认值在 config/config.go 的 DefaultConfig() 中设置
	
	// 仅兼容旧字段（如果旧字段有值而新字段为0，则迁移）
	if cfg.ProcessCPUThreshold > 0 && cfg.ProcCPUThreshold == 0 {
		cfg.ProcCPUThreshold = cfg.ProcessCPUThreshold
	}
	if cfg.ProcessMemoryThreshold > 0 && cfg.ProcMemoryThreshold == 0 {
		cfg.ProcMemoryThreshold = cfg.ProcessMemoryThreshold
	}
	if cfg.ProcessDiskIOThreshold > 0 && cfg.ProcDiskReadThreshold == 0 && cfg.ProcDiskWriteThreshold == 0 {
		cfg.ProcDiskReadThreshold = cfg.ProcessDiskIOThreshold
		cfg.ProcDiskWriteThreshold = cfg.ProcessDiskIOThreshold
	}
	if cfg.ProcessNetworkThreshold > 0 && cfg.ProcNetRecvThreshold == 0 && cfg.ProcNetSendThreshold == 0 {
		cfg.ProcNetRecvThreshold = cfg.ProcessNetworkThreshold
		cfg.ProcNetSendThreshold = cfg.ProcessNetworkThreshold
	}

	return &ImpactAnalyzer{
		provider:      prov,
		config:        cfg,
		targets:       getTargets,
		getProcesses:  getProcesses,
		stopCh:        make(chan struct{}),
		activeImpacts: make(map[impactKey]*types.ImpactEvent),
		fileChecker:   NewFileChecker(),
		portChecker:   NewPortChecker(),
		targetPorts:   make(map[int32][]int),
		targetFiles:   make(map[int32][]string),
	}
}

// Start 启动影响分析
func (a *ImpactAnalyzer) Start() {
	a.mu.Lock()
	if a.running || !a.config.Enabled {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	go a.loop()
	logger.Infof("IMPACT", "ImpactAnalyzer started (interval=%ds)", a.config.AnalysisInterval)
}

// Stop 停止影响分析
func (a *ImpactAnalyzer) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return
	}
	a.running = false
	close(a.stopCh)
	a.stopCh = make(chan struct{})
	logger.Info("IMPACT", "ImpactAnalyzer stopped")
}

// IsRunning 返回运行状态
func (a *ImpactAnalyzer) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// UpdateConfig 更新配置（运行时生效）
func (a *ImpactAnalyzer) UpdateConfig(cfg types.ImpactConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// 更新阈值配置
	if cfg.CPUThreshold > 0 {
		a.config.CPUThreshold = cfg.CPUThreshold
	}
	if cfg.MemoryThreshold > 0 {
		a.config.MemoryThreshold = cfg.MemoryThreshold
	}
	if cfg.DiskIOThreshold > 0 {
		a.config.DiskIOThreshold = cfg.DiskIOThreshold
	}
	if cfg.NetworkThreshold > 0 {
		a.config.NetworkThreshold = cfg.NetworkThreshold
	}
	if cfg.TopNProcesses > 0 {
		a.config.TopNProcesses = cfg.TopNProcesses
	}
	if cfg.AnalysisInterval > 0 {
		a.config.AnalysisInterval = cfg.AnalysisInterval
	}
	if cfg.FileCheckInterval > 0 {
		a.config.FileCheckInterval = cfg.FileCheckInterval
	}
	if cfg.PortCheckInterval > 0 {
		a.config.PortCheckInterval = cfg.PortCheckInterval
	}
	// 进程级别阈值（支持设为0以禁用检测）
	a.config.ProcCPUThreshold = cfg.ProcCPUThreshold
	a.config.ProcMemoryThreshold = cfg.ProcMemoryThreshold
	a.config.ProcMemGrowthThreshold = cfg.ProcMemGrowthThreshold
	a.config.ProcVMSThreshold = cfg.ProcVMSThreshold
	a.config.ProcFDsThreshold = cfg.ProcFDsThreshold
	a.config.ProcThreadsThreshold = cfg.ProcThreadsThreshold
	a.config.ProcOpenFilesThreshold = cfg.ProcOpenFilesThreshold
	a.config.ProcDiskReadThreshold = cfg.ProcDiskReadThreshold
	a.config.ProcDiskWriteThreshold = cfg.ProcDiskWriteThreshold
	a.config.ProcNetRecvThreshold = cfg.ProcNetRecvThreshold
	a.config.ProcNetSendThreshold = cfg.ProcNetSendThreshold
	
	logger.Infof("IMPACT", "Config updated: SysCPU=%.0f%%, SysMem=%.0f%%, ProcCPU=%.0f%%, ProcMem=%.0fMB",
		a.config.CPUThreshold, a.config.MemoryThreshold, a.config.ProcCPUThreshold, a.config.ProcMemoryThreshold)
}

// GetConfig 获取当前配置
func (a *ImpactAnalyzer) GetConfig() types.ImpactConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// SetEventCallback 设置事件回调函数
func (a *ImpactAnalyzer) SetEventCallback(cb EventCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.eventCallback = cb
}

// GetRecentImpacts 获取活跃的影响事件
func (a *ImpactAnalyzer) GetRecentImpacts(n int) []types.ImpactEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]types.ImpactEvent, 0, len(a.activeImpacts))
	for _, imp := range a.activeImpacts {
		result = append(result, *imp)
	}

	// 按时间排序（最新的在后）
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	if n > 0 && len(result) > n {
		result = result[len(result)-n:]
	}
	return result
}

// GetImpactSummary 获取影响统计摘要
func (a *ImpactAnalyzer) GetImpactSummary() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 按类型统计
	byType := make(map[string]int)
	bySeverity := make(map[string]int)
	byTarget := make(map[string]int)

	for _, imp := range a.activeImpacts {
		byType[imp.ImpactType]++
		bySeverity[imp.Severity]++
		byTarget[imp.TargetName]++
	}

	return map[string]interface{}{
		"total":       len(a.activeImpacts),
		"by_type":     byType,
		"by_severity": bySeverity,
		"by_target":   byTarget,
	}
}

// RemoveTargetEvents 删除特定目标的所有事件
func (a *ImpactAnalyzer) RemoveTargetEvents(targetPID int32) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key := range a.activeImpacts {
		if key.TargetPID == targetPID {
			delete(a.activeImpacts, key)
		}
	}
	logger.Infof("IMPACT", "Removed impact events for target PID %d", targetPID)
}

// ClearAllEvents 清除所有事件
func (a *ImpactAnalyzer) ClearAllEvents() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activeImpacts = make(map[impactKey]*types.ImpactEvent)
}

func (a *ImpactAnalyzer) loop() {
	ticker := time.NewTicker(time.Duration(a.config.AnalysisInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.analyze()
		}
	}
}

func (a *ImpactAnalyzer) analyze() {
	targets := a.targets()
	if len(targets) == 0 {
		// 没有监控目标，清除所有事件
		a.mu.Lock()
		a.activeImpacts = make(map[impactKey]*types.ImpactEvent)
		a.mu.Unlock()
		return
	}

	// 获取系统指标
	sysMetrics, err := a.provider.GetSystemMetrics()
	if err != nil {
		logger.Warnf("IMPACT", "Get system metrics failed: %v", err)
		return
	}

	// 获取所有进程
	processes, err := a.getProcesses()
	if err != nil {
		logger.Warnf("IMPACT", "List processes failed: %v", err)
		return
	}

	// 创建 PID -> ProcessInfo 映射
	procMap := make(map[int32]*types.ProcessInfo)
	for i := range processes {
		procMap[processes[i].PID] = &processes[i]
	}

	// 创建目标 PID 集合
	targetPIDSet := make(map[int32]bool)
	for _, t := range targets {
		targetPIDSet[t.PID] = true
	}

	// 分析各类影响（瞬时指标，每次先清除旧的同类型事件）
	a.analyzeCPU(sysMetrics, processes, targets, procMap, targetPIDSet)
	a.analyzeMemory(sysMetrics, processes, targets, procMap, targetPIDSet)
	a.analyzeDiskIO(sysMetrics, processes, targets, procMap, targetPIDSet)
	a.analyzeNetwork(sysMetrics, processes, targets, procMap, targetPIDSet)
	a.analyzeOtherMetrics(sysMetrics, processes, targets, procMap, targetPIDSet)

	// 低频检测：文件和端口冲突（动态维护）
	now := time.Now()
	if now.Sub(a.lastPortCheck) >= time.Duration(a.config.PortCheckInterval)*time.Second {
		a.analyzePortConflict(targets, procMap, targetPIDSet)
		a.lastPortCheck = now
	}
	if now.Sub(a.lastFileCheck) >= time.Duration(a.config.FileCheckInterval)*time.Second {
		a.analyzeFileConflict(targets, procMap, targetPIDSet)
		a.lastFileCheck = now
	}

	// 清理已不存在的目标的事件
	a.cleanupOrphanedEvents(targetPIDSet)
}

// cleanupOrphanedEvents 清理已不存在的目标的事件
func (a *ImpactAnalyzer) cleanupOrphanedEvents(targetPIDSet map[int32]bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key := range a.activeImpacts {
		if !targetPIDSet[key.TargetPID] {
			delete(a.activeImpacts, key)
		}
	}
}

// clearEventsByType 清除特定类型的所有事件
func (a *ImpactAnalyzer) clearEventsByType(impactType string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key := range a.activeImpacts {
		if key.ImpactType == impactType {
			delete(a.activeImpacts, key)
		}
	}
}

// analyzeCPU 分析 CPU 竞争
func (a *ImpactAnalyzer) analyzeCPU(
	sys *types.SystemMetrics,
	procs []types.ProcessInfo,
	targets []types.MonitorTarget,
	procMap map[int32]*types.ProcessInfo,
	targetPIDSet map[int32]bool,
) {
	// 先清除旧的 CPU 事件
	a.clearEventsByType("cpu")

	// 检查是否触发系统级别阈值
	systemTriggered := sys.CPUPercent >= a.config.CPUThreshold

	// 获取 Top N CPU 消耗进程
	topCPU := a.getTopByField(procs, "cpu", a.config.TopNProcesses)

	// 找出非目标的 CPU 消耗者
	for _, target := range targets {
		targetProc := procMap[target.PID]
		if targetProc == nil {
			continue
		}

		for _, proc := range topCPU {
			// 跳过目标自身
			if targetPIDSet[proc.PID] {
				continue
			}

			// 检查是否触发进程级别阈值
			processTriggered := a.config.ProcCPUThreshold > 0 && proc.CPUPct >= a.config.ProcCPUThreshold

			// 如果系统级别和进程级别都未触发，跳过
			if !systemTriggered && !processTriggered {
				continue
			}

			// 如果是系统级别触发，还需要进程 CPU > 10%
			if systemTriggered && !processTriggered && proc.CPUPct < 10 {
				continue
			}

			// 计算严重程度
			var severity string
			var description string
			if processTriggered {
				// 进程级别触发
				severity = a.getProcessSeverity(proc.CPUPct, a.config.ProcCPUThreshold)
				description = fmt.Sprintf("进程 %s (PID %d) CPU 占用 %.1f%% 超过阈值 %.0f%%", proc.Name, proc.PID, proc.CPUPct, a.config.ProcCPUThreshold)
			} else {
				// 系统级别触发
				severity = a.getSeverity(sys.CPUPercent, 80, 90, 95)
				description = fmt.Sprintf("系统 CPU %.1f%% 超过阈值，进程 %s (PID %d) 占用 %.1f%%", sys.CPUPercent, proc.Name, proc.PID, proc.CPUPct)
			}

			event := types.ImpactEvent{
				Timestamp:   time.Now(),
				TargetPID:   target.PID,
				TargetName:  a.getTargetDisplayName(target),
				ImpactType:  "cpu",
				Severity:    severity,
				SourcePID:   proc.PID,
				SourceName:  proc.Name,
				Description: description,
				Metrics: types.ImpactMetrics{
					SystemCPU:    sys.CPUPercent,
					SystemMemory: sys.MemoryPercent,
					TargetCPU:    targetProc.CPUPct,
					TargetMemory: targetProc.RSSBytes,
					SourceCPU:    proc.CPUPct,
					SourceMemory: proc.RSSBytes,
				},
				Suggestion: a.getCPUSuggestion(severity, proc.Name, proc.CPUPct),
			}
			a.recordImpact(event, "")
		}
	}
}

// analyzeMemory 分析内存压力
func (a *ImpactAnalyzer) analyzeMemory(
	sys *types.SystemMetrics,
	procs []types.ProcessInfo,
	targets []types.MonitorTarget,
	procMap map[int32]*types.ProcessInfo,
	targetPIDSet map[int32]bool,
) {
	// 先清除旧的 memory 事件
	a.clearEventsByType("memory")

	// 检查是否触发系统级别阈值
	systemTriggered := sys.MemoryPercent >= a.config.MemoryThreshold
	// 进程内存阈值转换为字节
	procMemThreshold := a.config.ProcMemoryThreshold * 1024 * 1024

	// 获取 Top N 内存消耗进程
	topMem := a.getTopByField(procs, "memory", a.config.TopNProcesses)

	for _, target := range targets {
		targetProc := procMap[target.PID]
		if targetProc == nil {
			continue
		}

		for _, proc := range topMem {
			if targetPIDSet[proc.PID] {
				continue
			}

			// 检查是否触发进程级别阈值
			processTriggered := a.config.ProcMemoryThreshold > 0 && float64(proc.RSSBytes) >= procMemThreshold

			// 如果系统级别和进程级别都未触发，跳过
			if !systemTriggered && !processTriggered {
				continue
			}

			// 如果是系统级别触发，还需要进程内存 > 100MB
			if systemTriggered && !processTriggered && proc.RSSBytes < 100*1024*1024 {
				continue
			}

			// 计算严重程度
			var severity string
			var description string
			if processTriggered {
				// 进程级别触发
				severity = a.getProcessSeverity(float64(proc.RSSBytes), procMemThreshold)
				description = fmt.Sprintf("进程 %s (PID %d) 内存占用 %s 超过阈值 %.0f MB", proc.Name, proc.PID, formatBytes(proc.RSSBytes), a.config.ProcMemoryThreshold)
			} else {
				// 系统级别触发
				severity = a.getSeverity(sys.MemoryPercent, 85, 92, 98)
				description = fmt.Sprintf("系统内存 %.1f%% 超过阈值，进程 %s (PID %d) 占用 %s", sys.MemoryPercent, proc.Name, proc.PID, formatBytes(proc.RSSBytes))
			}

			event := types.ImpactEvent{
				Timestamp:   time.Now(),
				TargetPID:   target.PID,
				TargetName:  a.getTargetDisplayName(target),
				ImpactType:  "memory",
				Severity:    severity,
				SourcePID:   proc.PID,
				SourceName:  proc.Name,
				Description: description,
				Metrics: types.ImpactMetrics{
					SystemCPU:    sys.CPUPercent,
					SystemMemory: sys.MemoryPercent,
					TargetCPU:    targetProc.CPUPct,
					TargetMemory: targetProc.RSSBytes,
					SourceCPU:    proc.CPUPct,
					SourceMemory: proc.RSSBytes,
				},
				Suggestion: a.getMemorySuggestion(severity, proc.Name, proc.RSSBytes, proc.RSSGrowthRate),
			}
			a.recordImpact(event, "")
		}
	}
}

// analyzeDiskIO 分析磁盘 IO 竞争
func (a *ImpactAnalyzer) analyzeDiskIO(
	sys *types.SystemMetrics,
	procs []types.ProcessInfo,
	targets []types.MonitorTarget,
	procMap map[int32]*types.ProcessInfo,
	targetPIDSet map[int32]bool,
) {
	// 先清除旧的 disk_io 事件
	a.clearEventsByType("disk_io")

	// 系统阈值转换为 B/s
	systemThreshold := a.config.DiskIOThreshold * 1024 * 1024
	totalIO := sys.DiskReadRate + sys.DiskWriteRate
	systemTriggered := totalIO >= systemThreshold

	// 进程阈值转换为 B/s
	procDiskReadThreshold := a.config.ProcDiskReadThreshold * 1024 * 1024
	procDiskWriteThreshold := a.config.ProcDiskWriteThreshold * 1024 * 1024

	// 获取 Top N 磁盘 IO 进程
	topIO := a.getTopByField(procs, "disk_io", a.config.TopNProcesses)

	for _, target := range targets {
		targetProc := procMap[target.PID]
		if targetProc == nil {
			continue
		}

		for _, proc := range topIO {
			if targetPIDSet[proc.PID] {
				continue
			}

			// 检查是否触发进程级别阈值（读或写）
			readTriggered := a.config.ProcDiskReadThreshold > 0 && proc.DiskReadRate >= procDiskReadThreshold
			writeTriggered := a.config.ProcDiskWriteThreshold > 0 && proc.DiskWriteRate >= procDiskWriteThreshold
			processTriggered := readTriggered || writeTriggered

			procIO := proc.DiskReadRate + proc.DiskWriteRate

			// 如果系统级别和进程级别都未触发，跳过
			if !systemTriggered && !processTriggered {
				continue
			}

			// 如果是系统级别触发，还需要进程 IO > 10MB/s
			if systemTriggered && !processTriggered && procIO < 10*1024*1024 {
				continue
			}

			// 计算严重程度
			var severity string
			var description string
			if processTriggered {
				// 进程级别触发
				if readTriggered {
					severity = a.getProcessSeverity(proc.DiskReadRate, procDiskReadThreshold)
					description = fmt.Sprintf("进程 %s (PID %d) 磁盘读 %.1f MB/s 超过阈值 %.0f MB/s", proc.Name, proc.PID, proc.DiskReadRate/1024/1024, a.config.ProcDiskReadThreshold)
				} else {
					severity = a.getProcessSeverity(proc.DiskWriteRate, procDiskWriteThreshold)
					description = fmt.Sprintf("进程 %s (PID %d) 磁盘写 %.1f MB/s 超过阈值 %.0f MB/s", proc.Name, proc.PID, proc.DiskWriteRate/1024/1024, a.config.ProcDiskWriteThreshold)
				}
			} else {
				// 系统级别触发
				severity = a.getSeverity(totalIO/1024/1024, 100, 200, 500)
				description = fmt.Sprintf("系统磁盘 IO %.1f MB/s 超过阈值，进程 %s (PID %d) IO 速率 %.1f MB/s", totalIO/1024/1024, proc.Name, proc.PID, procIO/1024/1024)
			}

			event := types.ImpactEvent{
				Timestamp:   time.Now(),
				TargetPID:   target.PID,
				TargetName:  a.getTargetDisplayName(target),
				ImpactType:  "disk_io",
				Severity:    severity,
				SourcePID:   proc.PID,
				SourceName:  proc.Name,
				Description: description,
				Metrics: types.ImpactMetrics{
					SystemCPU:    sys.CPUPercent,
					SystemMemory: sys.MemoryPercent,
					TargetCPU:    targetProc.CPUPct,
					TargetMemory: targetProc.RSSBytes,
					SourceCPU:    proc.CPUPct,
					SourceMemory: proc.RSSBytes,
					SourceDiskIO: procIO,
				},
				Suggestion: fmt.Sprintf("进程 %s 磁盘 IO 较高，可能导致监控目标 IO 延迟，建议检查该进程的 IO 操作", proc.Name),
			}
			a.recordImpact(event, "")
		}
	}
}

// analyzeNetwork 分析网络带宽竞争
func (a *ImpactAnalyzer) analyzeNetwork(
	sys *types.SystemMetrics,
	procs []types.ProcessInfo,
	targets []types.MonitorTarget,
	procMap map[int32]*types.ProcessInfo,
	targetPIDSet map[int32]bool,
) {
	// 先清除旧的 network 事件
	a.clearEventsByType("network")

	// 系统阈值转换为 B/s
	systemThreshold := a.config.NetworkThreshold * 1024 * 1024
	totalNet := sys.NetRecvRate + sys.NetSendRate
	systemTriggered := totalNet >= systemThreshold

	// 进程阈值转换为 B/s
	procNetRecvThreshold := a.config.ProcNetRecvThreshold * 1024 * 1024
	procNetSendThreshold := a.config.ProcNetSendThreshold * 1024 * 1024

	// 获取 Top N 网络流量进程
	topNet := a.getTopByField(procs, "network", a.config.TopNProcesses)

	for _, target := range targets {
		targetProc := procMap[target.PID]
		if targetProc == nil {
			continue
		}

		for _, proc := range topNet {
			if targetPIDSet[proc.PID] {
				continue
			}

			// 检查是否触发进程级别阈值（收或发）
			recvTriggered := a.config.ProcNetRecvThreshold > 0 && proc.NetRecvRate >= procNetRecvThreshold
			sendTriggered := a.config.ProcNetSendThreshold > 0 && proc.NetSendRate >= procNetSendThreshold
			processTriggered := recvTriggered || sendTriggered

			procNet := proc.NetRecvRate + proc.NetSendRate

			// 如果系统级别和进程级别都未触发，跳过
			if !systemTriggered && !processTriggered {
				continue
			}

			// 如果是系统级别触发，还需要进程网络 > 10MB/s
			if systemTriggered && !processTriggered && procNet < 10*1024*1024 {
				continue
			}

			// 计算严重程度
			var severity string
			var description string
			if processTriggered {
				// 进程级别触发
				if recvTriggered {
					severity = a.getProcessSeverity(proc.NetRecvRate, procNetRecvThreshold)
					description = fmt.Sprintf("进程 %s (PID %d) 网络收 %.1f MB/s 超过阈值 %.0f MB/s", proc.Name, proc.PID, proc.NetRecvRate/1024/1024, a.config.ProcNetRecvThreshold)
				} else {
					severity = a.getProcessSeverity(proc.NetSendRate, procNetSendThreshold)
					description = fmt.Sprintf("进程 %s (PID %d) 网络发 %.1f MB/s 超过阈值 %.0f MB/s", proc.Name, proc.PID, proc.NetSendRate/1024/1024, a.config.ProcNetSendThreshold)
				}
			} else {
				// 系统级别触发
				severity = "medium"
				description = fmt.Sprintf("系统网络流量 %.1f MB/s 超过阈值，进程 %s (PID %d) 流量 %.1f MB/s", totalNet/1024/1024, proc.Name, proc.PID, procNet/1024/1024)
			}

			event := types.ImpactEvent{
				Timestamp:   time.Now(),
				TargetPID:   target.PID,
				TargetName:  a.getTargetDisplayName(target),
				ImpactType:  "network",
				Severity:    severity,
				SourcePID:   proc.PID,
				SourceName:  proc.Name,
				Description: description,
				Metrics: types.ImpactMetrics{
					SystemCPU:    sys.CPUPercent,
					SystemMemory: sys.MemoryPercent,
					TargetCPU:    targetProc.CPUPct,
					TargetMemory: targetProc.RSSBytes,
					SourceCPU:    proc.CPUPct,
					SourceMemory: proc.RSSBytes,
					SourceNetIO:  procNet,
				},
				Suggestion: fmt.Sprintf("进程 %s 网络流量较高，可能影响监控目标的网络通信", proc.Name),
			}
			a.recordImpact(event, "")
		}
	}
}

// analyzePortConflict 分析端口占用冲突
// 自动获取监控目标的监听端口，检测其他进程是否尝试连接监控目标的端口
func (a *ImpactAnalyzer) analyzePortConflict(targets []types.MonitorTarget, procMap map[int32]*types.ProcessInfo, targetPIDSet map[int32]bool) {
	// 每 60 秒更新一次监控目标的监听端口缓存
	now := time.Now()
	if now.Sub(a.targetPortsTime) > 60*time.Second {
		a.refreshTargetPorts(targets)
		a.targetPortsTime = now
	}

	// 获取所有网络连接（一次性调用，减少开销）
	allConns, err := a.portChecker.getAllConnections()
	if err != nil {
		return
	}

	// 收集本次检测到的所有端口冲突 (使用 string key)
	currentConflicts := make(map[string]bool)

	for _, target := range targets {
		// 合并配置的 WatchPorts 和 自动发现的监听端口
		watchPorts := a.getWatchPortsForTarget(target)
		if len(watchPorts) == 0 {
			continue
		}

		// 检查是否有其他进程连接或监听监控目标的端口
		for _, port := range watchPorts {
			conflicts := a.findPortConflicts(allConns, port, target.PID, targetPIDSet)
			for _, conflict := range conflicts {
				conflictKey := fmt.Sprintf("%d-%d-%d", target.PID, conflict.PID, port)
				currentConflicts[conflictKey] = true

				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "port",
					Severity:    a.getPortConflictSeverity(conflict.Status),
					SourcePID:   conflict.PID,
					SourceName:  conflict.Name,
					Description: fmt.Sprintf("端口 %d %s，进程 %s (PID %d)", port, a.getPortStatusDesc(conflict.Status), conflict.Name, conflict.PID),
					Metrics: types.ImpactMetrics{
						ConflictPort: port,
					},
					Suggestion: a.getPortConflictSuggestion(port, conflict),
				}
				a.recordImpact(event, fmt.Sprintf("port:%d", port))
			}
		}
	}

	// 删除不再存在的端口冲突事件
	a.mu.Lock()
	var toRemove []impactKey
	for key := range a.activeImpacts {
		if key.ImpactType != "port" {
			continue
		}
		// 从 detail 中解析端口号
		var port int
		fmt.Sscanf(key.Detail, "port:%d", &port)
		conflictKey := fmt.Sprintf("%d-%d-%d", key.TargetPID, key.SourcePID, port)
		if !currentConflicts[conflictKey] {
			toRemove = append(toRemove, key)
		}
	}
	// 先收集要删除的事件
	removedEvents := make([]*types.ImpactEvent, 0, len(toRemove))
	for _, key := range toRemove {
		if evt := a.activeImpacts[key]; evt != nil {
			removedEvents = append(removedEvents, evt)
		}
		delete(a.activeImpacts, key)
	}
	a.mu.Unlock()

	// 记录移除事件
	for _, evt := range removedEvents {
		a.recordImpactRemoved(evt)
	}
}

// refreshTargetPorts 刷新监控目标的监听端口缓存
func (a *ImpactAnalyzer) refreshTargetPorts(targets []types.MonitorTarget) {
	a.targetPorts = make(map[int32][]int)
	for _, target := range targets {
		ports := a.portChecker.GetListeningPorts(target.PID)
		if len(ports) > 0 {
			a.targetPorts[target.PID] = ports
		}
	}
}

// getWatchPortsForTarget 获取目标需要监控的端口（配置 + 自动发现）
func (a *ImpactAnalyzer) getWatchPortsForTarget(target types.MonitorTarget) []int {
	portSet := make(map[int]bool)

	// 配置的 WatchPorts
	for _, p := range target.WatchPorts {
		portSet[p] = true
	}

	// 自动发现的监听端口
	if discovered, ok := a.targetPorts[target.PID]; ok {
		for _, p := range discovered {
			portSet[p] = true
		}
	}

	var ports []int
	for p := range portSet {
		ports = append(ports, p)
	}
	return ports
}

// findPortConflicts 查找端口冲突
func (a *ImpactAnalyzer) findPortConflicts(conns []ConnectionInfo, port int, excludePID int32, targetPIDs map[int32]bool) []PortConflict {
	var conflicts []PortConflict
	seen := make(map[int32]bool) // 避免同一进程重复报告

	for _, conn := range conns {
		// 检查是否涉及该端口
		if conn.LocalPort != port && conn.RemotePort != port {
			continue
		}

		// 排除监控目标自身
		if conn.PID == excludePID || conn.PID == 0 {
			continue
		}

		// 排除其他监控目标
		if targetPIDs[conn.PID] {
			continue
		}

		// 避免重复
		if seen[conn.PID] {
			continue
		}
		seen[conn.PID] = true

		// 只报告 LISTEN 冲突和 ESTABLISHED 连接
		if conn.Status != "LISTEN" && conn.Status != "ESTABLISHED" {
			continue
		}

		conflicts = append(conflicts, PortConflict{
			PID:    conn.PID,
			Name:   conn.ProcessName,
			Port:   port,
			Status: conn.Status,
		})
	}

	return conflicts
}

// getPortConflictSeverity 根据冲突类型返回严重程度
func (a *ImpactAnalyzer) getPortConflictSeverity(status string) string {
	if status == "LISTEN" {
		return "critical" // 端口被其他进程监听，严重冲突
	}
	return "medium" // 连接状态，中级
}

// getPortStatusDesc 获取端口状态描述
func (a *ImpactAnalyzer) getPortStatusDesc(status string) string {
	if status == "LISTEN" {
		return "被其他进程监听"
	}
	return "有外部连接"
}

// getPortConflictSuggestion 获取端口冲突建议
func (a *ImpactAnalyzer) getPortConflictSuggestion(port int, conflict PortConflict) string {
	if conflict.Status == "LISTEN" {
		return fmt.Sprintf("端口 %d 被 %s (PID %d) 监听，存在端口冲突，建议检查配置或终止冲突进程", port, conflict.Name, conflict.PID)
	}
	return fmt.Sprintf("进程 %s (PID %d) 正在连接监控目标的端口 %d", conflict.Name, conflict.PID, port)
}

// analyzeFileConflict 分析文件占用冲突
// 自动发现监控目标打开的文件，检测其他进程是否也打开了同样的文件
func (a *ImpactAnalyzer) analyzeFileConflict(targets []types.MonitorTarget, procMap map[int32]*types.ProcessInfo, targetPIDSet map[int32]bool) {
	now := time.Now()

	// 每 60 秒更新一次监控目标的打开文件缓存
	if now.Sub(a.targetFilesTime) > 60*time.Second {
		a.refreshTargetFiles(targets)
		a.targetFilesTime = now
	}

	// 刷新所有进程的打开文件缓存
	a.fileChecker.RefreshOpenFiles(targetPIDSet)

	// 收集本次检测到的所有文件冲突
	currentConflicts := make(map[string]bool)

	// 检测每个监控目标的文件冲突
	for _, target := range targets {
		// 合并配置的 WatchFiles 和 自动发现的打开文件
		watchFiles := a.getWatchFilesForTarget(target)
		if len(watchFiles) == 0 {
			continue
		}

		// 查找冲突
		conflicts := a.fileChecker.FindConflicts(target.PID, watchFiles, targetPIDSet)
		for _, conflict := range conflicts {
			conflictKey := fmt.Sprintf("%d-%d-%s", target.PID, conflict.PID, conflict.Path)
			currentConflicts[conflictKey] = true

			event := types.ImpactEvent{
				Timestamp:   time.Now(),
				TargetPID:   target.PID,
				TargetName:  a.getTargetDisplayName(target),
				ImpactType:  "file",
				Severity:    "high",
				SourcePID:   conflict.PID,
				SourceName:  conflict.Name,
				Description: fmt.Sprintf("文件 %s 被进程 %s (PID %d) 同时打开", conflict.Path, conflict.Name, conflict.PID),
				Metrics: types.ImpactMetrics{
					ConflictFile: conflict.Path,
				},
				Suggestion: fmt.Sprintf("文件 %s 被多个进程打开，可能影响监控目标对该文件的独占访问", conflict.Path),
			}
			a.recordImpact(event, "file:"+conflict.Path)
		}
	}

	// 删除不再存在的文件冲突事件
	a.mu.Lock()
	var toRemove []impactKey
	for key := range a.activeImpacts {
		if key.ImpactType != "file" {
			continue
		}
		// 从 detail 中提取文件路径
		filePath := ""
		if len(key.Detail) > 5 && key.Detail[:5] == "file:" {
			filePath = key.Detail[5:]
		}
		conflictKey := fmt.Sprintf("%d-%d-%s", key.TargetPID, key.SourcePID, filePath)
		if !currentConflicts[conflictKey] {
			toRemove = append(toRemove, key)
		}
	}
	// 先收集要删除的事件
	removedEvents := make([]*types.ImpactEvent, 0, len(toRemove))
	for _, key := range toRemove {
		if evt := a.activeImpacts[key]; evt != nil {
			removedEvents = append(removedEvents, evt)
		}
		delete(a.activeImpacts, key)
	}
	a.mu.Unlock()

	// 记录移除事件
	for _, evt := range removedEvents {
		a.recordImpactRemoved(evt)
	}
}

// refreshTargetFiles 刷新监控目标的打开文件缓存
func (a *ImpactAnalyzer) refreshTargetFiles(targets []types.MonitorTarget) {
	a.targetFiles = make(map[int32][]string)
	for _, target := range targets {
		files := a.fileChecker.GetFilesOpenedByPID(target.PID)
		if len(files) > 0 {
			a.targetFiles[target.PID] = files
		}
	}
}

// getWatchFilesForTarget 获取目标需要监控的文件（配置 + 自动发现）
func (a *ImpactAnalyzer) getWatchFilesForTarget(target types.MonitorTarget) []string {
	fileSet := make(map[string]bool)

	// 配置的 WatchFiles
	for _, f := range target.WatchFiles {
		fileSet[f] = true
	}

	// 自动发现的打开文件
	if discovered, ok := a.targetFiles[target.PID]; ok {
		for _, f := range discovered {
			fileSet[f] = true
		}
	}

	var files []string
	for f := range fileSet {
		files = append(files, f)
	}
	return files
}

// 辅助函数

func (a *ImpactAnalyzer) recordImpact(event types.ImpactEvent, detail string) {
	key := impactKey{
		TargetPID:  event.TargetPID,
		ImpactType: event.ImpactType,
		SourcePID:  event.SourcePID,
		Detail:     detail,
	}

	a.mu.Lock()
	_, exists := a.activeImpacts[key]
	a.activeImpacts[key] = &event
	callback := a.eventCallback
	a.mu.Unlock()

	if !exists {
		logger.Impact(event.ImpactType, event.Severity, event.TargetName, event.SourceName, event.Description)

		// 记录到事件日志
		if callback != nil {
			eventType := "impact_" + event.ImpactType
			message := fmt.Sprintf("[影响%s] %s → %s: %s",
				a.getSeverityName(event.Severity), event.SourceName, event.TargetName, event.Description)
			callback(eventType, event.SourcePID, event.SourceName, message)
		}
	}
}

// recordImpactRemoved 记录影响事件移除
func (a *ImpactAnalyzer) recordImpactRemoved(event *types.ImpactEvent) {
	a.mu.RLock()
	callback := a.eventCallback
	a.mu.RUnlock()

	if callback != nil {
		eventType := "impact_resolved"
		message := fmt.Sprintf("[影响解除] %s 对 %s 的 %s 影响已解除",
			event.SourceName, event.TargetName, a.getImpactTypeName(event.ImpactType))
		callback(eventType, event.SourcePID, event.SourceName, message)
	}
}

func (a *ImpactAnalyzer) getSeverityName(severity string) string {
	switch severity {
	case "critical":
		return "严重"
	case "high":
		return "高级"
	case "medium":
		return "中级"
	default:
		return "低级"
	}
}

func (a *ImpactAnalyzer) getImpactTypeName(impactType string) string {
	switch impactType {
	case "cpu":
		return "CPU竞争"
	case "memory":
		return "内存压力"
	case "mem_growth":
		return "内存增速"
	case "disk_io":
		return "磁盘IO"
	case "network":
		return "网络带宽"
	case "file":
		return "文件占用"
	case "port":
		return "端口占用"
	case "fds":
		return "句柄数"
	case "threads":
		return "线程数"
	case "open_files":
		return "打开文件数"
	case "vms":
		return "虚拟内存"
	default:
		return impactType
	}
}

func (a *ImpactAnalyzer) getTargetDisplayName(target types.MonitorTarget) string {
	if target.Alias != "" {
		return target.Alias
	}
	return target.Name
}

func (a *ImpactAnalyzer) getSeverity(value float64, low, medium, high float64) string {
	if value >= high {
		return "critical"
	}
	if value >= medium {
		return "high"
	}
	if value >= low {
		return "medium"
	}
	return "low"
}

// getProcessSeverity 根据进程指标超过阈值的程度计算严重性
func (a *ImpactAnalyzer) getProcessSeverity(value, threshold float64) string {
	ratio := value / threshold
	if ratio >= 2.0 {
		return "critical"
	}
	if ratio >= 1.5 {
		return "high"
	}
	return "medium"
}

func (a *ImpactAnalyzer) getCPUSuggestion(severity, procName string, cpuPct float64) string {
	switch severity {
	case "critical":
		return fmt.Sprintf("CPU 资源严重不足，进程 %s 占用 %.1f%%，建议立即检查该进程或增加系统资源", procName, cpuPct)
	case "high":
		return fmt.Sprintf("进程 %s 占用大量 CPU (%.1f%%)，可能影响监控目标性能，建议检查该进程是否正常", procName, cpuPct)
	default:
		return fmt.Sprintf("建议关注进程 %s，CPU 占用 %.1f%%", procName, cpuPct)
	}
}

func (a *ImpactAnalyzer) getMemorySuggestion(severity, procName string, rss uint64, growthRate float64) string {
	if growthRate > 1024*1024 { // > 1MB/s 增长
		return fmt.Sprintf("进程 %s 内存持续增长 (+%.1f MB/s)，可能存在内存泄漏，建议检查", procName, growthRate/1024/1024)
	}
	switch severity {
	case "critical":
		return fmt.Sprintf("内存即将耗尽，进程 %s 占用 %s，存在 OOM 风险，建议立即处理", procName, formatBytes(rss))
	case "high":
		return fmt.Sprintf("内存压力较大，进程 %s 占用 %s，建议检查是否可以释放", procName, formatBytes(rss))
	default:
		return fmt.Sprintf("建议关注进程 %s 的内存使用 (%s)", procName, formatBytes(rss))
	}
}

func (a *ImpactAnalyzer) getTopByField(procs []types.ProcessInfo, field string, n int) []types.ProcessInfo {
	sorted := make([]types.ProcessInfo, len(procs))
	copy(sorted, procs)

	switch field {
	case "cpu":
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].CPUPct > sorted[j].CPUPct })
	case "memory":
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].RSSBytes > sorted[j].RSSBytes })
	case "disk_io":
		sort.Slice(sorted, func(i, j int) bool {
			return (sorted[i].DiskReadRate + sorted[i].DiskWriteRate) > (sorted[j].DiskReadRate + sorted[j].DiskWriteRate)
		})
	case "network":
		sort.Slice(sorted, func(i, j int) bool {
			return (sorted[i].NetRecvRate + sorted[i].NetSendRate) > (sorted[j].NetRecvRate + sorted[j].NetSendRate)
		})
	}

	if len(sorted) > n {
		return sorted[:n]
	}
	return sorted
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// analyzeOtherMetrics 分析其他进程指标（内存增速、句柄数、线程数、打开文件数、虚拟内存）
func (a *ImpactAnalyzer) analyzeOtherMetrics(
	sys *types.SystemMetrics,
	procs []types.ProcessInfo,
	targets []types.MonitorTarget,
	procMap map[int32]*types.ProcessInfo,
	targetPIDSet map[int32]bool,
) {
	// 清除旧的其他类型事件
	a.clearEventsByType("mem_growth")
	a.clearEventsByType("fds")
	a.clearEventsByType("threads")
	a.clearEventsByType("open_files")
	a.clearEventsByType("vms")

	// 阈值转换
	memGrowthThreshold := a.config.ProcMemGrowthThreshold * 1024 * 1024 // MB/s -> B/s
	vmsThreshold := a.config.ProcVMSThreshold * 1024 * 1024             // MB -> B

	for _, target := range targets {
		targetProc := procMap[target.PID]
		if targetProc == nil {
			continue
		}

		for _, proc := range procs {
			// 跳过目标自身
			if targetPIDSet[proc.PID] {
				continue
			}

			// 检查内存增速
			if a.config.ProcMemGrowthThreshold > 0 && proc.RSSGrowthRate >= memGrowthThreshold {
				severity := a.getProcessSeverity(proc.RSSGrowthRate, memGrowthThreshold)
				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "mem_growth",
					Severity:    severity,
					SourcePID:   proc.PID,
					SourceName:  proc.Name,
					Description: fmt.Sprintf("进程 %s (PID %d) 内存增速 %.1f MB/s 超过阈值 %.0f MB/s", proc.Name, proc.PID, proc.RSSGrowthRate/1024/1024, a.config.ProcMemGrowthThreshold),
					Metrics: types.ImpactMetrics{
						SystemCPU:    sys.CPUPercent,
						SystemMemory: sys.MemoryPercent,
						SourceMemory: proc.RSSBytes,
					},
					Suggestion: fmt.Sprintf("进程 %s 内存持续增长，可能存在内存泄漏，建议检查", proc.Name),
				}
				a.recordImpact(event, "")
			}

			// 检查句柄数
			if a.config.ProcFDsThreshold > 0 && proc.NumFDs >= int32(a.config.ProcFDsThreshold) {
				severity := a.getProcessSeverity(float64(proc.NumFDs), float64(a.config.ProcFDsThreshold))
				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "fds",
					Severity:    severity,
					SourcePID:   proc.PID,
					SourceName:  proc.Name,
					Description: fmt.Sprintf("进程 %s (PID %d) 句柄数 %d 超过阈值 %d", proc.Name, proc.PID, proc.NumFDs, a.config.ProcFDsThreshold),
					Metrics: types.ImpactMetrics{
						SystemCPU:    sys.CPUPercent,
						SystemMemory: sys.MemoryPercent,
					},
					Suggestion: fmt.Sprintf("进程 %s 句柄数过高，可能存在资源泄漏，建议检查", proc.Name),
				}
				a.recordImpact(event, "")
			}

			// 检查线程数
			if a.config.ProcThreadsThreshold > 0 && proc.NumThreads >= int32(a.config.ProcThreadsThreshold) {
				severity := a.getProcessSeverity(float64(proc.NumThreads), float64(a.config.ProcThreadsThreshold))
				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "threads",
					Severity:    severity,
					SourcePID:   proc.PID,
					SourceName:  proc.Name,
					Description: fmt.Sprintf("进程 %s (PID %d) 线程数 %d 超过阈值 %d", proc.Name, proc.PID, proc.NumThreads, a.config.ProcThreadsThreshold),
					Metrics: types.ImpactMetrics{
						SystemCPU:    sys.CPUPercent,
						SystemMemory: sys.MemoryPercent,
					},
					Suggestion: fmt.Sprintf("进程 %s 线程数过多，可能影响系统性能，建议检查", proc.Name),
				}
				a.recordImpact(event, "")
			}

			// 检查打开文件数
			if a.config.ProcOpenFilesThreshold > 0 && proc.OpenFiles >= a.config.ProcOpenFilesThreshold {
				severity := a.getProcessSeverity(float64(proc.OpenFiles), float64(a.config.ProcOpenFilesThreshold))
				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "open_files",
					Severity:    severity,
					SourcePID:   proc.PID,
					SourceName:  proc.Name,
					Description: fmt.Sprintf("进程 %s (PID %d) 打开文件数 %d 超过阈值 %d", proc.Name, proc.PID, proc.OpenFiles, a.config.ProcOpenFilesThreshold),
					Metrics: types.ImpactMetrics{
						SystemCPU:    sys.CPUPercent,
						SystemMemory: sys.MemoryPercent,
					},
					Suggestion: fmt.Sprintf("进程 %s 打开文件数过多，可能影响系统性能", proc.Name),
				}
				a.recordImpact(event, "")
			}

			// 检查虚拟内存
			if a.config.ProcVMSThreshold > 0 && float64(proc.VMS) >= vmsThreshold {
				severity := a.getProcessSeverity(float64(proc.VMS), vmsThreshold)
				event := types.ImpactEvent{
					Timestamp:   time.Now(),
					TargetPID:   target.PID,
					TargetName:  a.getTargetDisplayName(target),
					ImpactType:  "vms",
					Severity:    severity,
					SourcePID:   proc.PID,
					SourceName:  proc.Name,
					Description: fmt.Sprintf("进程 %s (PID %d) 虚拟内存 %s 超过阈值 %.0f MB", proc.Name, proc.PID, formatBytes(proc.VMS), a.config.ProcVMSThreshold),
					Metrics: types.ImpactMetrics{
						SystemCPU:    sys.CPUPercent,
						SystemMemory: sys.MemoryPercent,
						SourceMemory: proc.VMS,
					},
					Suggestion: fmt.Sprintf("进程 %s 虚拟内存占用过高", proc.Name),
				}
				a.recordImpact(event, "")
			}
		}
	}
}
