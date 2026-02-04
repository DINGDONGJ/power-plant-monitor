package provider

import (
	"fmt"
	"sync"
	"time"

	"monitor-agent/netmon"
	"monitor-agent/types"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// 磁盘 IO 采样状态
type ioSample struct {
	readBytes  uint64
	writeBytes uint64
	readCount  uint64
	writeCount uint64
	sampleTime time.Time
	// 上次计算的速率
	lastReadRate  float64
	lastWriteRate float64
	lastReadOps   float64
	lastWriteOps  float64
}

// RSS 采样状态（用于计算增长速率）
type rssSample struct {
	rss        uint64
	sampleTime time.Time
	growthRate float64
}

// 进程 CPU 采样状态
type cpuSample struct {
	cpuTime    float64 // 累计 CPU 时间（秒）
	sampleTime time.Time
	lastPct    float64 // 上次计算的 CPU 百分比
}

// 系统级采样状态
type systemSample struct {
	// CPU 累计时间（用于增量计算）
	cpuUser    float64
	cpuSystem  float64
	cpuIdle    float64
	cpuIowait  float64
	cpuNice    float64
	cpuIrq     float64
	cpuSoftirq float64
	cpuSteal   float64
	cpuTotal   float64 // 累计总时间

	// CPU 百分比（计算结果）
	cpuUserPct   float64
	cpuSystemPct float64
	cpuIdlePct   float64
	cpuIowaitPct float64
	cpuTotalPct  float64

	// Swap 采样
	swapIn      uint64
	swapOut     uint64
	swapInRate  float64
	swapOutRate float64

	// 磁盘 IO 采样
	diskReadBytes  uint64
	diskWriteBytes uint64
	diskReadCount  uint64
	diskWriteCount uint64
	diskReadRate   float64
	diskWriteRate  float64
	diskReadOps    float64
	diskWriteOps   float64

	sampleTime time.Time
}

// processListCache 进程列表缓存
type processListCache struct {
	processes  []types.ProcessInfo
	cacheTime  time.Time
	cacheTTL   time.Duration
}

// commonProvider 通用 provider 实现
type commonProvider struct {
	// 进程级采样
	ioSamplesMu  sync.RWMutex
	ioSamples    map[int32]*ioSample
	rssSamplesMu sync.RWMutex
	rssSamples   map[int32]*rssSample
	cpuSamplesMu sync.RWMutex
	cpuSamples   map[int32]*cpuSample

	// 系统级采样缓存
	sysSampleMu sync.RWMutex
	sysSample   *systemSample

	// 进程列表缓存（避免短时间内多次请求返回不同数据）
	procCacheMu sync.RWMutex
	procCache   *processListCache

	// 监听端口缓存
	listenPortsMu    sync.RWMutex
	listenPorts      map[int32][]int
	listenPortsTime  time.Time

	// 进程网络监控
	netMonitor *netmon.NetMonitor

	// CPU 核心数（用于计算进程 CPU 百分比）
	numCPU int

	// 是否将进程 CPU 除以核心数（Windows 风格 = true，Linux 风格 = false）
	divideByNumCPU bool

	// 平台特定函数
	matchProcessName   func(procName, targetName string) bool
	formatCmdline      func(exe string) string
	getHandleCount     func(pid int32) int32
	getPriority        func(pid int32) int32
	getFileDescription func(exePath string) string
}

// newCommonProvider 创建通用 provider
// divideByNumCPU: Windows 风格设为 true（进程CPU最大100%），Linux 风格设为 false（单核100%，可超过100%）
func newCommonProvider(
	matchName func(procName, targetName string) bool,
	fmtCmdline func(exe string) string,
	getHandles func(pid int32) int32,
	getPrio func(pid int32) int32,
	getFileDesc func(exePath string) string,
	divideByNumCPU bool,
) *commonProvider {
	numCPU, _ := cpu.Counts(true)
	if numCPU == 0 {
		numCPU = 1
	}

	p := &commonProvider{
		ioSamples:          make(map[int32]*ioSample),
		rssSamples:         make(map[int32]*rssSample),
		cpuSamples:         make(map[int32]*cpuSample),
		sysSample:          &systemSample{sampleTime: time.Now()},
		procCache:          &processListCache{cacheTTL: 500 * time.Millisecond}, // 500ms 缓存
		listenPorts:        make(map[int32][]int),
		numCPU:             numCPU,
		divideByNumCPU:     divideByNumCPU,
		matchProcessName:   matchName,
		formatCmdline:      fmtCmdline,
		getHandleCount:     getHandles,
		getPriority:        getPrio,
		getFileDescription: getFileDesc,
		netMonitor:         netmon.New(),
	}

	// 初始化系统 CPU 采样
	p.initSystemCPUSample()

	go p.sampleSystemMetrics()

	// 启动进程网络监控
	if err := p.netMonitor.Start(); err != nil {
		fmt.Printf("[Provider] 进程网络监控启动失败: %v\n", err)
	}

	return p
}

// initSystemCPUSample 初始化系统 CPU 采样基准值
func (p *commonProvider) initSystemCPUSample() {
	cpuTimes, err := cpu.Times(false)
	if err != nil || len(cpuTimes) == 0 {
		return
	}

	t := cpuTimes[0]
	p.sysSampleMu.Lock()
	p.sysSample.cpuUser = t.User
	p.sysSample.cpuSystem = t.System
	p.sysSample.cpuIdle = t.Idle
	p.sysSample.cpuIowait = t.Iowait
	p.sysSample.cpuNice = t.Nice
	p.sysSample.cpuIrq = t.Irq
	p.sysSample.cpuSoftirq = t.Softirq
	p.sysSample.cpuSteal = t.Steal
	p.sysSample.cpuTotal = t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	p.sysSample.sampleTime = time.Now()
	p.sysSampleMu.Unlock()
}

// sampleSystemMetrics 后台定时采集系统指标
func (p *commonProvider) sampleSystemMetrics() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.collectSystemSample()
	}
}

// collectSystemSample 采集一次系统指标
func (p *commonProvider) collectSystemSample() {
	now := time.Now()

	// CPU 时间采样
	cpuTimes, _ := cpu.Times(false)

	// Swap 指标
	swapInfo, _ := mem.SwapMemory()
	var swapIn, swapOut uint64
	if swapInfo != nil {
		swapIn = swapInfo.Sin
		swapOut = swapInfo.Sout
	}

	// 系统磁盘 IO
	diskStats, _ := disk.IOCounters()
	var diskReadBytes, diskWriteBytes, diskReadCount, diskWriteCount uint64
	for _, stat := range diskStats {
		diskReadBytes += stat.ReadBytes
		diskWriteBytes += stat.WriteBytes
		diskReadCount += stat.ReadCount
		diskWriteCount += stat.WriteCount
	}

	// 计算速率
	p.sysSampleMu.Lock()
	defer p.sysSampleMu.Unlock()

	deltaTime := now.Sub(p.sysSample.sampleTime).Seconds()
	if deltaTime > 0.1 {
		// CPU 增量计算
		if len(cpuTimes) > 0 {
			t := cpuTimes[0]
			currentTotal := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal

			deltaTotal := currentTotal - p.sysSample.cpuTotal
			if deltaTotal > 0 {
				deltaUser := t.User - p.sysSample.cpuUser
				deltaSystem := t.System - p.sysSample.cpuSystem
				deltaIdle := t.Idle - p.sysSample.cpuIdle
				deltaIowait := t.Iowait - p.sysSample.cpuIowait

				p.sysSample.cpuUserPct = deltaUser / deltaTotal * 100
				p.sysSample.cpuSystemPct = deltaSystem / deltaTotal * 100
				p.sysSample.cpuIdlePct = deltaIdle / deltaTotal * 100
				p.sysSample.cpuIowaitPct = deltaIowait / deltaTotal * 100
				p.sysSample.cpuTotalPct = 100 - p.sysSample.cpuIdlePct
			}

			// 更新累计值
			p.sysSample.cpuUser = t.User
			p.sysSample.cpuSystem = t.System
			p.sysSample.cpuIdle = t.Idle
			p.sysSample.cpuIowait = t.Iowait
			p.sysSample.cpuNice = t.Nice
			p.sysSample.cpuIrq = t.Irq
			p.sysSample.cpuSoftirq = t.Softirq
			p.sysSample.cpuSteal = t.Steal
			p.sysSample.cpuTotal = currentTotal
		}

		// Swap 速率
		p.sysSample.swapInRate = float64(swapIn-p.sysSample.swapIn) / deltaTime
		p.sysSample.swapOutRate = float64(swapOut-p.sysSample.swapOut) / deltaTime

		// 磁盘 IO 速率
		p.sysSample.diskReadRate = float64(diskReadBytes-p.sysSample.diskReadBytes) / deltaTime
		p.sysSample.diskWriteRate = float64(diskWriteBytes-p.sysSample.diskWriteBytes) / deltaTime
		p.sysSample.diskReadOps = float64(diskReadCount-p.sysSample.diskReadCount) / deltaTime
		p.sysSample.diskWriteOps = float64(diskWriteCount-p.sysSample.diskWriteCount) / deltaTime
	}

	// 更新采样值
	p.sysSample.swapIn = swapIn
	p.sysSample.swapOut = swapOut
	p.sysSample.diskReadBytes = diskReadBytes
	p.sysSample.diskWriteBytes = diskWriteBytes
	p.sysSample.diskReadCount = diskReadCount
	p.sysSample.diskWriteCount = diskWriteCount
	p.sysSample.sampleTime = now
}

func (p *commonProvider) FindAllPIDsByName(name string) ([]int32, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	var pids []int32
	for _, proc := range procs {
		n, _ := proc.Name()
		if p.matchProcessName(n, name) {
			pids = append(pids, proc.Pid)
		}
	}
	return pids, nil
}

func (p *commonProvider) FindPIDByName(name string) (int32, error) {
	pids, err := p.FindAllPIDsByName(name)
	if err != nil {
		return 0, err
	}
	if len(pids) == 0 {
		return 0, fmt.Errorf("process %s not found", name)
	}
	if len(pids) > 1 {
		return 0, fmt.Errorf("multiple processes found with name %s: %v, please use -pid to specify", name, pids)
	}
	return pids[0], nil
}

func (p *commonProvider) GetMetrics(pid int32) (*types.ProcessMetrics, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	cpuPct := p.calcProcessCPU(pid, proc)
	memInfo, _ := proc.MemoryInfo()
	name, _ := proc.Name()

	var rss uint64
	if memInfo != nil {
		rss = memInfo.RSS
	}

	return &types.ProcessMetrics{
		PID:      pid,
		Name:     name,
		CPUPct:   cpuPct,
		RSSBytes: rss,
		Alive:    true,
	}, nil
}

func (p *commonProvider) IsAlive(pid int32) bool {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}
	running, _ := proc.IsRunning()
	return running
}

// calcDiskIO 计算进程磁盘 IO 速率
func (p *commonProvider) calcDiskIO(pid int32, readBytes, writeBytes, readCount, writeCount uint64) (readRate, writeRate, readOps, writeOps float64) {
	now := time.Now()

	p.ioSamplesMu.Lock()
	defer p.ioSamplesMu.Unlock()

	sample, exists := p.ioSamples[pid]
	if !exists {
		p.ioSamples[pid] = &ioSample{
			readBytes:  readBytes,
			writeBytes: writeBytes,
			readCount:  readCount,
			writeCount: writeCount,
			sampleTime: now,
		}
		return 0, 0, 0, 0
	}

	deltaTime := now.Sub(sample.sampleTime).Seconds()
	if deltaTime < 0.1 {
		return sample.lastReadRate, sample.lastWriteRate, sample.lastReadOps, sample.lastWriteOps
	}

	readRate = float64(readBytes-sample.readBytes) / deltaTime
	writeRate = float64(writeBytes-sample.writeBytes) / deltaTime
	readOps = float64(readCount-sample.readCount) / deltaTime
	writeOps = float64(writeCount-sample.writeCount) / deltaTime

	sample.readBytes = readBytes
	sample.writeBytes = writeBytes
	sample.readCount = readCount
	sample.writeCount = writeCount
	sample.sampleTime = now
	sample.lastReadRate = readRate
	sample.lastWriteRate = writeRate
	sample.lastReadOps = readOps
	sample.lastWriteOps = writeOps

	return readRate, writeRate, readOps, writeOps
}

// calcRSSGrowth 计算 RSS 增长速率
func (p *commonProvider) calcRSSGrowth(pid int32, rss uint64) float64 {
	now := time.Now()

	p.rssSamplesMu.Lock()
	defer p.rssSamplesMu.Unlock()

	sample, exists := p.rssSamples[pid]
	if !exists {
		p.rssSamples[pid] = &rssSample{
			rss:        rss,
			sampleTime: now,
		}
		return 0
	}

	deltaTime := now.Sub(sample.sampleTime).Seconds()
	if deltaTime < 0.5 {
		return sample.growthRate
	}

	// 计算增长速率（可能为负数表示内存释放）
	growthRate := float64(int64(rss)-int64(sample.rss)) / deltaTime

	sample.rss = rss
	sample.sampleTime = now
	sample.growthRate = growthRate

	return growthRate
}

// calcProcessCPU 计算进程 CPU 使用率（增量方式）
func (p *commonProvider) calcProcessCPU(pid int32, proc *process.Process) float64 {
	now := time.Now()

	// 获取进程 CPU 时间
	times, err := proc.Times()
	if err != nil {
		return 0
	}
	currentCPUTime := times.User + times.System

	p.cpuSamplesMu.Lock()
	defer p.cpuSamplesMu.Unlock()

	sample, exists := p.cpuSamples[pid]
	if !exists {
		p.cpuSamples[pid] = &cpuSample{
			cpuTime:    currentCPUTime,
			sampleTime: now,
			lastPct:    0,
		}
		return 0
	}

	deltaTime := now.Sub(sample.sampleTime).Seconds()
	if deltaTime < 0.1 {
		return sample.lastPct
	}

	// 计算 CPU 百分比：(CPU时间增量 / 实际时间增量) * 100
	deltaCPU := currentCPUTime - sample.cpuTime
	cpuPct := (deltaCPU / deltaTime) * 100

	// Windows 风格：除以核心数，最大 100%
	// Linux 风格：不除，单核 100%，多核可超过 100%
	if p.divideByNumCPU && p.numCPU > 0 {
		cpuPct = cpuPct / float64(p.numCPU)
	}

	// 更新采样
	sample.cpuTime = currentCPUTime
	sample.sampleTime = now
	sample.lastPct = cpuPct

	return cpuPct
}

func (p *commonProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
	// 检查缓存是否有效
	p.procCacheMu.RLock()
	if p.procCache.processes != nil && time.Since(p.procCache.cacheTime) < p.procCache.cacheTTL {
		result := make([]types.ProcessInfo, len(p.procCache.processes))
		copy(result, p.procCache.processes)
		p.procCacheMu.RUnlock()
		return result, nil
	}
	p.procCacheMu.RUnlock()

	// 缓存过期，重新采集
	result, err := p.collectAllProcesses()
	if err != nil {
		return nil, err
	}

	// 更新缓存
	p.procCacheMu.Lock()
	p.procCache.processes = make([]types.ProcessInfo, len(result))
	copy(p.procCache.processes, result)
	p.procCache.cacheTime = time.Now()
	p.procCacheMu.Unlock()

	return result, nil
}

// collectAllProcesses 实际采集所有进程信息
func (p *commonProvider) collectAllProcesses() ([]types.ProcessInfo, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}

	// 获取所有网络连接，用于统计每个进程的监听端口
	listenPorts := p.getProcessListenPorts()

	alivePids := make(map[int32]bool)
	var result []types.ProcessInfo

	for _, proc := range procs {
		alivePids[proc.Pid] = true

		name, _ := proc.Name()
		memInfo, _ := proc.MemoryInfo()
		status, _ := proc.Status()
		username, _ := proc.Username()
		cmdline, _ := proc.Cmdline()
		ioCounters, _ := proc.IOCounters()
		createTime, _ := proc.CreateTime()

		// 使用增量方式计算进程 CPU
		cpuPct := p.calcProcessCPU(proc.Pid, proc)

		// 获取句柄数/文件描述符数
		var numFDs int32
		if p.getHandleCount != nil {
			numFDs = p.getHandleCount(proc.Pid)
		} else {
			numFDs, _ = proc.NumFDs()
		}

		// 获取线程数
		numThreads, _ := proc.NumThreads()

		// 获取优先级和 Nice 值
		var priority int32
		var nice int32
		if p.getPriority != nil {
			priority = p.getPriority(proc.Pid)
		} else {
			niceVal, err := proc.Nice()
			if err == nil {
				nice = niceVal
				// Linux: 将 nice 值转换为优先级 (20 - nice)
				priority = 20 - niceVal
			}
		}

		// 获取可执行文件路径
		exePath, _ := proc.Exe()

		// 如果 cmdline 为空，尝试获取可执行文件路径
		if cmdline == "" {
			if exePath != "" {
				cmdline = p.formatCmdline(exePath)
			}
		}

		// 获取文件描述信息
		var description string
		if p.getFileDescription != nil && exePath != "" {
			description = p.getFileDescription(exePath)
		}

		var rss, vms uint64
		if memInfo != nil {
			rss = memInfo.RSS
			vms = memInfo.VMS
		}

		statusStr := ""
		if len(status) > 0 {
			statusStr = status[0]
		}

		// 计算磁盘 IO 速率
		var diskIO, diskReadRate, diskWriteRate, diskReadOps, diskWriteOps float64
		if ioCounters != nil {
			diskReadRate, diskWriteRate, diskReadOps, diskWriteOps = p.calcDiskIO(
				proc.Pid,
				ioCounters.ReadBytes, ioCounters.WriteBytes,
				ioCounters.ReadCount, ioCounters.WriteCount,
			)
			diskIO = diskReadRate + diskWriteRate
		}

		// 计算 RSS 增长速率
		rssGrowthRate := p.calcRSSGrowth(proc.Pid, rss)

		// 计算已运行时间（秒）
		var uptime int64
		if createTime > 0 {
			uptime = (time.Now().UnixMilli() - createTime) / 1000
		}

		// 获取进程网络流量
		var netRecvRate, netSendRate float64
		if p.netMonitor != nil {
			netStats := p.netMonitor.GetStats(proc.Pid)
			netRecvRate = netStats.RecvRate
			netSendRate = netStats.SendRate
		}

		// 获取进程打开的文件数（使用 NumFDs 作为代理）
		openFiles := int(numFDs)

		// 获取进程监听的端口
		var ports []int
		if p, ok := listenPorts[proc.Pid]; ok {
			ports = p
		}

		result = append(result, types.ProcessInfo{
			PID:           proc.Pid,
			Name:          name,
			CPUPct:        cpuPct,
			RSSBytes:      rss,
			RSSGrowthRate: rssGrowthRate,
			VMS:           vms,
			Status:        statusStr,
			Username:      username,
			NumFDs:        numFDs,
			NumThreads:    numThreads,
			Priority:      priority,
			Nice:          nice,
			DiskIO:        diskIO,
			DiskReadRate:  diskReadRate,
			DiskWriteRate: diskWriteRate,
			DiskReadOps:   diskReadOps,
			DiskWriteOps:  diskWriteOps,
			NetRecvRate:   netRecvRate,
			NetSendRate:   netSendRate,
			Uptime:        uptime,
			Cmdline:       cmdline,
			Description:   description,
			OpenFiles:     openFiles,
			ListenPorts:   ports,
		})
	}

	// 清理已退出进程的采样数据
	p.ioSamplesMu.Lock()
	for pid := range p.ioSamples {
		if !alivePids[pid] {
			delete(p.ioSamples, pid)
		}
	}
	p.ioSamplesMu.Unlock()

	p.rssSamplesMu.Lock()
	for pid := range p.rssSamples {
		if !alivePids[pid] {
			delete(p.rssSamples, pid)
		}
	}
	p.rssSamplesMu.Unlock()

	p.cpuSamplesMu.Lock()
	for pid := range p.cpuSamples {
		if !alivePids[pid] {
			delete(p.cpuSamples, pid)
		}
	}
	p.cpuSamplesMu.Unlock()

	// 清理 netmon 中的进程统计
	if p.netMonitor != nil {
		p.netMonitor.CleanupPids(alivePids)
	}

	return result, nil
}

// getProcessListenPorts 获取所有进程的监听端口（带缓存，3秒更新一次）
func (p *commonProvider) getProcessListenPorts() map[int32][]int {
	p.listenPortsMu.RLock()
	if time.Since(p.listenPortsTime) < 3*time.Second && len(p.listenPorts) > 0 {
		// 返回缓存的副本
		result := make(map[int32][]int, len(p.listenPorts))
		for k, v := range p.listenPorts {
			result[k] = v
		}
		p.listenPortsMu.RUnlock()
		return result
	}
	p.listenPortsMu.RUnlock()

	// 缓存过期，重新获取
	conns, err := psnet.Connections("all")
	if err != nil {
		return p.listenPorts
	}

	p.listenPortsMu.Lock()
	defer p.listenPortsMu.Unlock()

	// 清空并复用 map
	for k := range p.listenPorts {
		delete(p.listenPorts, k)
	}

	for _, conn := range conns {
		if conn.Status == "LISTEN" && conn.Pid != 0 {
			port := int(conn.Laddr.Port)
			p.listenPorts[conn.Pid] = append(p.listenPorts[conn.Pid], port)
		}
	}
	p.listenPortsTime = time.Now()

	// 返回副本
	result := make(map[int32][]int, len(p.listenPorts))
	for k, v := range p.listenPorts {
		result[k] = v
	}
	return result
}

func (p *commonProvider) GetSystemMetrics() (*types.SystemMetrics, error) {
	// 内存指标
	memInfo, _ := mem.VirtualMemory()
	swapInfo, _ := mem.SwapMemory()

	// 系统负载 (Linux)
	var loadAvg1, loadAvg5, loadAvg15 float64
	if loadStat, err := load.Avg(); err == nil && loadStat != nil {
		loadAvg1 = loadStat.Load1
		loadAvg5 = loadStat.Load5
		loadAvg15 = loadStat.Load15
	}

	// 获取缓存的系统采样
	p.sysSampleMu.RLock()
	cpuTotal := p.sysSample.cpuTotalPct
	cpuUser := p.sysSample.cpuUserPct
	cpuSystem := p.sysSample.cpuSystemPct
	cpuIowait := p.sysSample.cpuIowaitPct
	cpuIdle := p.sysSample.cpuIdlePct
	swapInRate := p.sysSample.swapInRate
	swapOutRate := p.sysSample.swapOutRate
	diskReadRate := p.sysSample.diskReadRate
	diskWriteRate := p.sysSample.diskWriteRate
	diskReadOps := p.sysSample.diskReadOps
	diskWriteOps := p.sysSample.diskWriteOps
	p.sysSampleMu.RUnlock()

	// 网络流量
	var netRecv, netSent uint64
	var netRecvRate, netSendRate float64
	if p.netMonitor != nil {
		sysStats := p.netMonitor.GetSystemStats()
		netRecv = sysStats.RecvBytes
		netSent = sysStats.SendBytes
		netRecvRate = sysStats.RecvRate
		netSendRate = sysStats.SendRate
	}

	// Swap 指标
	var swapTotal, swapUsed uint64
	var swapPercent float64
	if swapInfo != nil {
		swapTotal = swapInfo.Total
		swapUsed = swapInfo.Used
		swapPercent = swapInfo.UsedPercent
	}

	return &types.SystemMetrics{
		// CPU
		CPUPercent: cpuTotal,
		CPUUser:    cpuUser,
		CPUSystem:  cpuSystem,
		CPUIowait:  cpuIowait,
		CPUIdle:    cpuIdle,

		// 负载 (Linux)
		LoadAvg1:  loadAvg1,
		LoadAvg5:  loadAvg5,
		LoadAvg15: loadAvg15,

		// 内存
		MemoryTotal:     memInfo.Total,
		MemoryUsed:      memInfo.Used,
		MemoryAvailable: memInfo.Available,
		MemoryPercent:   memInfo.UsedPercent,

		// Swap
		SwapTotal:   swapTotal,
		SwapUsed:    swapUsed,
		SwapPercent: swapPercent,
		SwapInRate:  swapInRate,
		SwapOutRate: swapOutRate,

		// 网络
		NetBytesRecv: netRecv,
		NetBytesSent: netSent,
		NetRecvRate:  netRecvRate,
		NetSendRate:  netSendRate,

		// 磁盘 IO
		DiskReadRate:  diskReadRate,
		DiskWriteRate: diskWriteRate,
		DiskReadOps:   diskReadOps,
		DiskWriteOps:  diskWriteOps,
	}, nil
}
