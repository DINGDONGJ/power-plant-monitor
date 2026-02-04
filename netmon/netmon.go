package netmon

import (
	"log"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

// ProcessNetStats 进程网络统计
type ProcessNetStats struct {
	RecvBytes uint64
	SendBytes uint64
	RecvRate  float64
	SendRate  float64
}

// SystemNetStats 系统网络统计
type SystemNetStats struct {
	RecvBytes uint64
	SendBytes uint64
	RecvRate  float64
	SendRate  float64
}

// NetMonitor 网络流量监控器
type NetMonitor struct {
	mu sync.RWMutex

	// 进程网络统计
	stats map[int32]*processNetSample

	// 系统总流量统计
	sysStats *systemNetSample

	// 上次系统流量（用于计算增量）
	lastSysRecv uint64
	lastSysSend uint64

	// 进程连接数缓存（减少 net.Connections 调用频率）
	procConnCount map[int32]int
	totalConns    int
	connCacheTime time.Time

	// 运行状态
	running bool
	stopCh  chan struct{}
}

type processNetSample struct {
	recvBytes  uint64
	sendBytes  uint64
	recvRate   float64
	sendRate   float64
}

type systemNetSample struct {
	recvBytes uint64
	sendBytes uint64
	recvRate  float64
	sendRate  float64
}

// New 创建网络监控器
func New() *NetMonitor {
	return &NetMonitor{
		stats:         make(map[int32]*processNetSample),
		sysStats:      &systemNetSample{},
		procConnCount: make(map[int32]int),
		stopCh:        make(chan struct{}),
	}
}

// Start 启动网络监控
func (m *NetMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	go m.collectLoop()

	log.Printf("[NetMon] 网络监控已启动（gopsutil）")
	return nil
}

// Stop 停止网络监控
func (m *NetMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()
}

// GetStats 获取进程网络统计
func (m *NetMonitor) GetStats(pid int32) *ProcessNetStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sample, ok := m.stats[pid]
	if !ok {
		return &ProcessNetStats{}
	}

	return &ProcessNetStats{
		RecvBytes: sample.recvBytes,
		SendBytes: sample.sendBytes,
		RecvRate:  sample.recvRate,
		SendRate:  sample.sendRate,
	}
}

// GetSystemStats 获取系统网络统计
func (m *NetMonitor) GetSystemStats() *SystemNetStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &SystemNetStats{
		RecvBytes: m.sysStats.recvBytes,
		SendBytes: m.sysStats.sendBytes,
		RecvRate:  m.sysStats.recvRate,
		SendRate:  m.sysStats.sendRate,
	}
}

// GetAllStats 获取所有进程的网络统计
func (m *NetMonitor) GetAllStats() map[int32]*ProcessNetStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[int32]*ProcessNetStats)
	for pid, sample := range m.stats {
		result[pid] = &ProcessNetStats{
			RecvBytes: sample.recvBytes,
			SendBytes: sample.sendBytes,
			RecvRate:  sample.recvRate,
			SendRate:  sample.sendRate,
		}
	}
	return result
}

// IsRunning 检查是否运行中
func (m *NetMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// CleanupPids 清理不存在的进程统计
func (m *NetMonitor) CleanupPids(alivePids map[int32]bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for pid := range m.stats {
		if !alivePids[pid] {
			delete(m.stats, pid)
		}
	}
}

// collectLoop 采集循环
func (m *NetMonitor) collectLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.collect()
		}
	}
}

// collect 采集一次数据
func (m *NetMonitor) collect() {
	// 获取系统网络统计
	counters, err := net.IOCounters(false)
	if err != nil || len(counters) == 0 {
		return
	}

	var totalRecv, totalSend uint64
	for _, c := range counters {
		totalRecv += c.BytesRecv
		totalSend += c.BytesSent
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 每 3 秒更新一次连接数缓存（net.Connections 开销大）
	now := time.Now()
	if now.Sub(m.connCacheTime) >= 3*time.Second {
		connections, _ := net.Connections("all")
		
		// 清空并复用 map
		for k := range m.procConnCount {
			delete(m.procConnCount, k)
		}
		m.totalConns = 0

		for _, conn := range connections {
			if conn.Pid > 0 {
				m.procConnCount[int32(conn.Pid)]++
				m.totalConns++
			}
		}
		m.connCacheTime = now
	}

	// 计算系统流量增量
	var recvDelta, sendDelta uint64
	if m.lastSysRecv > 0 {
		if totalRecv >= m.lastSysRecv {
			recvDelta = totalRecv - m.lastSysRecv
		}
		if totalSend >= m.lastSysSend {
			sendDelta = totalSend - m.lastSysSend
		}
	}

	// 更新系统统计
	m.sysStats.recvRate = float64(recvDelta)
	m.sysStats.sendRate = float64(sendDelta)
	m.sysStats.recvBytes = totalRecv
	m.sysStats.sendBytes = totalSend
	m.lastSysRecv = totalRecv
	m.lastSysSend = totalSend

	// 按连接数比例分配增量给各进程
	if m.totalConns > 0 && (recvDelta > 0 || sendDelta > 0) {
		for pid, count := range m.procConnCount {
			ratio := float64(count) / float64(m.totalConns)

			sample, ok := m.stats[pid]
			if !ok {
				sample = &processNetSample{}
				m.stats[pid] = sample
			}

			// 累加增量
			procRecv := uint64(float64(recvDelta) * ratio)
			procSend := uint64(float64(sendDelta) * ratio)

			sample.recvBytes += procRecv
			sample.sendBytes += procSend
			sample.recvRate = float64(procRecv)
			sample.sendRate = float64(procSend)
		}
	}
}
