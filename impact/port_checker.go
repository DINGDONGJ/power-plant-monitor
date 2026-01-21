package impact

import (
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// PortConflict 端口冲突信息
type PortConflict struct {
	PID    int32
	Name   string
	Port   int
	Status string // LISTEN, ESTABLISHED, etc.
}

// ConnectionInfo 网络连接信息
type ConnectionInfo struct {
	PID         int32
	ProcessName string
	LocalPort   int
	RemotePort  int
	Status      string
}

// PortChecker 端口占用检测器
type PortChecker struct {
	// 进程名缓存，避免频繁查询
	procNameCache map[int32]string
}

// NewPortChecker 创建端口检测器
func NewPortChecker() *PortChecker {
	return &PortChecker{
		procNameCache: make(map[int32]string),
	}
}

// getAllConnections 获取所有网络连接（一次性调用，减少开销）
func (c *PortChecker) getAllConnections() ([]ConnectionInfo, error) {
	conns, err := net.Connections("all")
	if err != nil {
		return nil, err
	}

	var result []ConnectionInfo
	for _, conn := range conns {
		if conn.Pid == 0 {
			continue
		}

		// 获取进程名（带缓存）
		procName := c.getProcessName(conn.Pid)

		result = append(result, ConnectionInfo{
			PID:         conn.Pid,
			ProcessName: procName,
			LocalPort:   int(conn.Laddr.Port),
			RemotePort:  int(conn.Raddr.Port),
			Status:      conn.Status,
		})
	}

	return result, nil
}

// getProcessName 获取进程名（带缓存）
func (c *PortChecker) getProcessName(pid int32) string {
	if name, ok := c.procNameCache[pid]; ok {
		return name
	}

	name := "unknown"
	if proc, err := process.NewProcess(pid); err == nil {
		if n, err := proc.Name(); err == nil {
			name = n
		}
	}

	// 缓存不超过 500 个，避免内存无限增长
	if len(c.procNameCache) < 500 {
		c.procNameCache[pid] = name
	}

	return name
}

// CheckPort 检查指定端口是否被其他进程占用
// 返回占用该端口的进程列表（排除 excludePID）
func (c *PortChecker) CheckPort(port int, excludePID int32) []PortConflict {
	var conflicts []PortConflict

	// 获取所有网络连接
	conns, err := net.Connections("all")
	if err != nil {
		return conflicts
	}

	for _, conn := range conns {
		// 检查本地端口是否匹配
		if conn.Laddr.Port != uint32(port) {
			continue
		}

		// 排除指定进程
		if conn.Pid == excludePID {
			continue
		}

		// 忽略 PID 为 0 的连接（内核连接）
		if conn.Pid == 0 {
			continue
		}

		// 获取进程名称
		procName := "unknown"
		if proc, err := process.NewProcess(conn.Pid); err == nil {
			if name, err := proc.Name(); err == nil {
				procName = name
			}
		}

		conflicts = append(conflicts, PortConflict{
			PID:    conn.Pid,
			Name:   procName,
			Port:   port,
			Status: conn.Status,
		})
	}

	return conflicts
}

// CheckPorts 批量检查多个端口
func (c *PortChecker) CheckPorts(ports []int, excludePID int32) map[int][]PortConflict {
	result := make(map[int][]PortConflict)

	// 获取所有网络连接（只调用一次）
	conns, err := net.Connections("all")
	if err != nil {
		return result
	}

	// 创建端口集合以便快速查找
	portSet := make(map[int]bool)
	for _, p := range ports {
		portSet[p] = true
		result[p] = []PortConflict{}
	}

	// 缓存进程名称
	procNames := make(map[int32]string)

	for _, conn := range conns {
		port := int(conn.Laddr.Port)
		if !portSet[port] {
			continue
		}

		if conn.Pid == excludePID || conn.Pid == 0 {
			continue
		}

		// 获取进程名称（带缓存）
		procName, ok := procNames[conn.Pid]
		if !ok {
			procName = "unknown"
			if proc, err := process.NewProcess(conn.Pid); err == nil {
				if name, err := proc.Name(); err == nil {
					procName = name
				}
			}
			procNames[conn.Pid] = procName
		}

		result[port] = append(result[port], PortConflict{
			PID:    conn.Pid,
			Name:   procName,
			Port:   port,
			Status: conn.Status,
		})
	}

	return result
}

// GetListeningPorts 获取指定进程监听的所有端口
func (c *PortChecker) GetListeningPorts(pid int32) []int {
	var ports []int

	conns, err := net.Connections("all")
	if err != nil {
		return ports
	}

	for _, conn := range conns {
		if conn.Pid == pid && conn.Status == "LISTEN" {
			ports = append(ports, int(conn.Laddr.Port))
		}
	}

	return ports
}
