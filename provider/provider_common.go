package provider

import (
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"monitor-agent/netmon"
	"monitor-agent/types"
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

// 系统级采样状态
type systemSample struct {
	// CPU 详细指标
	cpuUser   float64
	cpuSystem float64
	cpuIowait float64
	cpuIdle   float64
	cpuTotal  float64

	// Swap 采样
	swapIn     uint64
	swapOut    uint64
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

// commonProvider 通用 provider 实现
type commonProvider struct {
	// 进程级采样
	ioSamplesMu  sync.RWMutex
	ioSamples    map[int32]*ioSample
	rssSamplesMu sync.RWMutex
	rssSamples   map[int32]*rssSample

	// 系统级采样缓存
	sysSampleMu sync.RWMutex
	sysSample   *systemSample

	// 进程网络监控
	netMonitor *netmon.NetMonitor

	// 平台特定函数
	matchProcessName func(procName, targetName string) bool
	formatCmdline    func(exe string) string
	getHandleCount   func(pid int32) int32
	getPriority      func(pid int32) int32
}

// newCommonProvider 创建通用 provider
func newCommonProvider(
	matchName func(procName, targetName string) bool,
	fmtCmdline func(exe string) string,
	getHandles func(pid int32) int32,
	getPrio func(pid int32) int32,
) *commonProvider {
	p := &commonProvider{
		ioSamples:        make(map[int32]*ioSample),
		rssSamples:       make(map[int32]*rssSample),
		sysSample:        &systemSample{sampleTime: time.Now()},
		matchProcessName: matchName,
		formatCmdline:    fmtCmdline,
		getHandleCount:   getHandles,
		getPriority:      getPrio,
		netMonitor:       netmon.New(),
	}
	go p.sampleSystemMetrics()

	// 启动进程网络监控
	if err := p.netMonitor.Start(); err != nil {
		fmt.Printf("[Provider] 进程网络监控启动失败: %v\n", err)
	}

	return p
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

	// CPU 详细指标
	cpuTimes, _ := cpu.Times(false) // false = 合并所有 CPU
	var cpuUser, cpuSystem, cpuIowait, cpuIdle, cpuTotal float64
	if len(cpuTimes) > 0 {
		t := cpuTimes[0]
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
		if total > 0 {
			cpuUser = t.User / total * 100
			cpuSystem = t.System / total * 100
			cpuIowait = t.Iowait / total * 100
			cpuIdle = t.Idle / total * 100
			cpuTotal = 100 - cpuIdle
		}
	}

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
	p.sysSample.cpuUser = cpuUser
	p.sysSample.cpuSystem = cpuSystem
	p.sysSample.cpuIowait = cpuIowait
	p.sysSample.cpuIdle = cpuIdle
	p.sysSample.cpuTotal = cpuTotal
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
	cpuPct, _ := proc.CPUPercent()
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

func (p *commonProvider) ListAllProcesses() ([]types.ProcessInfo, error) {
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
		cpuPct, _ := proc.CPUPercent()
		memInfo, _ := proc.MemoryInfo()
		status, _ := proc.Status()
		username, _ := proc.Username()
		cmdline, _ := proc.Cmdline()
		ioCounters, _ := proc.IOCounters()
		createTime, _ := proc.CreateTime()

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

		// 如果 cmdline 为空，尝试获取可执行文件路径
		if cmdline == "" {
			if exe, err := proc.Exe(); err == nil && exe != "" {
				cmdline = p.formatCmdline(exe)
			}
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

	return result, nil
}

// getProcessListenPorts 获取所有进程的监听端口
func (p *commonProvider) getProcessListenPorts() map[int32][]int {
	result := make(map[int32][]int)

	conns, err := psnet.Connections("all")
	if err != nil {
		return result
	}

	for _, conn := range conns {
		if conn.Status == "LISTEN" && conn.Pid != 0 {
			port := int(conn.Laddr.Port)
			result[conn.Pid] = append(result[conn.Pid], port)
		}
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
	cpuTotal := p.sysSample.cpuTotal
	cpuUser := p.sysSample.cpuUser
	cpuSystem := p.sysSample.cpuSystem
	cpuIowait := p.sysSample.cpuIowait
	cpuIdle := p.sysSample.cpuIdle
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
