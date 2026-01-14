package netmon

import (
	"log"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
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

	// 端口到 PID 的映射
	portToPID map[uint16]int32

	// 进程网络统计
	stats map[int32]*processNetSample

	// 系统总流量统计
	sysStats *systemNetSample

	// 抓包句柄
	handles []*pcap.Handle

	// 运行状态
	running bool
	stopCh  chan struct{}
}

type processNetSample struct {
	recvBytes  uint64
	sendBytes  uint64
	sampleTime time.Time
	recvRate   float64
	sendRate   float64
}

type systemNetSample struct {
	recvBytes  uint64
	sendBytes  uint64
	sampleTime time.Time
	recvRate   float64
	sendRate   float64
}

// New 创建网络监控器
func New() *NetMonitor {
	return &NetMonitor{
		portToPID: make(map[uint16]int32),
		stats:     make(map[int32]*processNetSample),
		sysStats:  &systemNetSample{sampleTime: time.Now()},
		stopCh:    make(chan struct{}),
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

	// 获取所有网络接口
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Printf("[NetMon] 获取网络接口失败: %v", err)
		return err
	}

	// 在每个有 IP 地址的接口上启动抓包
	for _, device := range devices {
		if len(device.Addresses) == 0 {
			continue
		}

		// 跳过 loopback
		isLoopback := false
		for _, addr := range device.Addresses {
			if addr.IP.IsLoopback() {
				isLoopback = true
				break
			}
		}
		if isLoopback {
			continue
		}

		handle, err := pcap.OpenLive(device.Name, 65535, true, pcap.BlockForever)
		if err != nil {
			log.Printf("[NetMon] 打开接口 %s 失败: %v", device.Name, err)
			continue
		}

		// 只抓 TCP 和 UDP
		err = handle.SetBPFFilter("tcp or udp")
		if err != nil {
			log.Printf("[NetMon] 设置过滤器失败: %v", err)
			handle.Close()
			continue
		}

		m.handles = append(m.handles, handle)
		go m.capturePackets(handle, device.Name)
		log.Printf("[NetMon] 开始监控接口: %s", device.Name)
	}

	// 定期更新端口映射
	go m.updatePortMapping()

	// 定期计算速率
	go m.calculateRates()

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

	for _, handle := range m.handles {
		handle.Close()
	}
	m.handles = nil
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

// capturePackets 抓包处理
func (m *NetMonitor) capturePackets(handle *pcap.Handle, deviceName string) {
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	for {
		select {
		case <-m.stopCh:
			return
		case packet, ok := <-packetSource.Packets():
			if !ok {
				return
			}
			m.processPacket(packet)
		}
	}
}

// processPacket 处理单个数据包
func (m *NetMonitor) processPacket(packet gopacket.Packet) {
	// 获取网络层
	networkLayer := packet.NetworkLayer()
	if networkLayer == nil {
		return
	}

	// 获取传输层
	transportLayer := packet.TransportLayer()
	if transportLayer == nil {
		return
	}

	var srcPort, dstPort uint16
	var packetLen int

	// 解析端口
	switch t := transportLayer.(type) {
	case *layers.TCP:
		srcPort = uint16(t.SrcPort)
		dstPort = uint16(t.DstPort)
	case *layers.UDP:
		srcPort = uint16(t.SrcPort)
		dstPort = uint16(t.DstPort)
	default:
		return
	}

	packetLen = len(packet.Data())

	m.mu.Lock()
	defer m.mu.Unlock()

	// 判断是发送还是接收（通过检查源端口是否属于本机进程）
	_, isSrcLocal := m.portToPID[srcPort]
	_, isDstLocal := m.portToPID[dstPort]

	if isSrcLocal {
		// 源端口是本机进程 -> 发送
		m.sysStats.sendBytes += uint64(packetLen)
		pid := m.portToPID[srcPort]
		if m.stats[pid] == nil {
			m.stats[pid] = &processNetSample{sampleTime: time.Now()}
		}
		m.stats[pid].sendBytes += uint64(packetLen)
	}

	if isDstLocal {
		// 目标端口是本机进程 -> 接收
		m.sysStats.recvBytes += uint64(packetLen)
		pid := m.portToPID[dstPort]
		if m.stats[pid] == nil {
			m.stats[pid] = &processNetSample{sampleTime: time.Now()}
		}
		m.stats[pid].recvBytes += uint64(packetLen)
	}
}

// updatePortMapping 更新端口到 PID 的映射
func (m *NetMonitor) updatePortMapping() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.refreshPortMapping()
		}
	}
}

// refreshPortMapping 刷新端口映射
func (m *NetMonitor) refreshPortMapping() {
	// 获取所有网络连接
	connections, err := net.Connections("all")
	if err != nil {
		return
	}

	newMapping := make(map[uint16]int32)
	for _, conn := range connections {
		if conn.Pid > 0 && conn.Laddr.Port > 0 {
			newMapping[uint16(conn.Laddr.Port)] = conn.Pid
		}
	}

	m.mu.Lock()
	m.portToPID = newMapping
	m.mu.Unlock()
}

// calculateRates 计算速率
func (m *NetMonitor) calculateRates() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 保存上一次的统计
	lastStats := make(map[int32]struct {
		recvBytes uint64
		sendBytes uint64
		time      time.Time
	})

	var lastSysRecv, lastSysSent uint64
	var lastSysTime time.Time

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()

			// 计算系统总流量速率
			if !lastSysTime.IsZero() {
				deltaTime := now.Sub(lastSysTime).Seconds()
				if deltaTime > 0 {
					m.sysStats.recvRate = float64(m.sysStats.recvBytes-lastSysRecv) / deltaTime
					m.sysStats.sendRate = float64(m.sysStats.sendBytes-lastSysSent) / deltaTime
				}
			}
			lastSysRecv = m.sysStats.recvBytes
			lastSysSent = m.sysStats.sendBytes
			lastSysTime = now

			// 计算进程流量速率
			for pid, sample := range m.stats {
				last, ok := lastStats[pid]
				if ok {
					deltaTime := now.Sub(last.time).Seconds()
					if deltaTime > 0 {
						sample.recvRate = float64(sample.recvBytes-last.recvBytes) / deltaTime
						sample.sendRate = float64(sample.sendBytes-last.sendBytes) / deltaTime
					}
				}

				lastStats[pid] = struct {
					recvBytes uint64
					sendBytes uint64
					time      time.Time
				}{
					recvBytes: sample.recvBytes,
					sendBytes: sample.sendBytes,
					time:      now,
				}
			}

			// 清理不存在的进程
			for pid := range m.stats {
				if _, err := process.NewProcess(pid); err != nil {
					delete(m.stats, pid)
					delete(lastStats, pid)
				}
			}

			m.mu.Unlock()
		}
	}
}

// IsRunning 检查是否运行中
func (m *NetMonitor) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}
