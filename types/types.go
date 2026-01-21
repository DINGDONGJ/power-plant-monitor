package types

import "time"

// ProcessMetrics 进程指标
type ProcessMetrics struct {
	Timestamp time.Time `json:"timestamp"`
	PID       int32     `json:"pid"`
	Name      string    `json:"name"`
	CPUPct    float64   `json:"cpu_pct"`
	RSSBytes  uint64    `json:"rss_bytes"`
	Alive     bool      `json:"alive"`
}

// Event 事件记录
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "exit", "start", "new_process", "process_gone"
	PID       int32     `json:"pid"`
	Name      string    `json:"name"`
	Message   string    `json:"message"`
}

// ProcessChange 进程变化记录
type ProcessChange struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "new" 或 "gone"
	PID       int32     `json:"pid"`
	Name      string    `json:"name"`
	Cmdline   string    `json:"cmdline,omitempty"`
}

// ProcessInfo 系统进程信息（用于列表展示）
type ProcessInfo struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	CPUPct        float64 `json:"cpu_pct"`
	RSSBytes      uint64  `json:"rss_bytes"`
	RSSGrowthRate float64 `json:"rss_growth_rate"` // RSS 增长速率 (B/s)
	VMS           uint64  `json:"vms"`             // 虚拟内存大小
	Status        string  `json:"status"`
	Username      string  `json:"username"`        // 发布者/用户
	NumFDs        int32   `json:"num_fds"`         // 句柄数/文件描述符数
	NumThreads    int32   `json:"num_threads"`     // 线程数
	Priority      int32   `json:"priority"`        // 进程优先级
	Nice          int32   `json:"nice"`            // Nice 值 (Linux)
	DiskIO        float64 `json:"disk_io"`         // 磁盘速率 (B/s) - 保留兼容
	DiskReadRate  float64 `json:"disk_read_rate"`  // 磁盘读取速率 (B/s)
	DiskWriteRate float64 `json:"disk_write_rate"` // 磁盘写入速率 (B/s)
	DiskReadOps   float64 `json:"disk_read_ops"`   // 磁盘读取次数/秒
	DiskWriteOps  float64 `json:"disk_write_ops"`  // 磁盘写入次数/秒
	NetRecvRate   float64 `json:"net_recv_rate"`   // 网络接收速率 (B/s)
	NetSendRate   float64 `json:"net_send_rate"`   // 网络发送速率 (B/s)
	Uptime        int64   `json:"uptime"`          // 已运行时间（秒）
	Cmdline       string  `json:"cmdline"`         // 命令行
	OpenFiles     int     `json:"open_files"`      // 打开的文件数
	ListenPorts   []int   `json:"listen_ports"`    // 监听的端口列表
}

// MonitorTarget 监控目标
type MonitorTarget struct {
	PID        int32    `json:"pid"`
	Name       string   `json:"name"`                  // 进程名
	Alias      string   `json:"alias,omitempty"`       // 备注名称（如：电力监控主进程）
	Cmdline    string   `json:"cmdline,omitempty"`
	WatchFiles []string `json:"watch_files,omitempty"` // 需要监控的关键文件路径
	WatchPorts []int    `json:"watch_ports,omitempty"` // 需要监控的端口列表
}

// MultiMonitorConfig 多进程监控配置
type MultiMonitorConfig struct {
	Targets          []MonitorTarget `json:"targets"`
	SampleInterval   int             `json:"sample_interval"` // 采样间隔（秒）
	MetricsBufferLen int             `json:"metrics_buffer_len"`
	EventsBufferLen  int             `json:"events_buffer_len"`
	LogDir           string          `json:"log_dir"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	// CPU 指标
	CPUPercent float64 `json:"cpu_percent"`
	CPUUser    float64 `json:"cpu_user"`    // 用户态 CPU%
	CPUSystem  float64 `json:"cpu_system"`  // 内核态 CPU%
	CPUIowait  float64 `json:"cpu_iowait"`  // IO 等待 CPU%
	CPUIdle    float64 `json:"cpu_idle"`    // 空闲 CPU%

	// 负载指标 (Linux)
	LoadAvg1  float64 `json:"load_avg_1"`  // 1 分钟负载
	LoadAvg5  float64 `json:"load_avg_5"`  // 5 分钟负载
	LoadAvg15 float64 `json:"load_avg_15"` // 15 分钟负载

	// 内存指标
	MemoryTotal     uint64  `json:"memory_total"`
	MemoryUsed      uint64  `json:"memory_used"`
	MemoryAvailable uint64  `json:"memory_available"` // 可用内存
	MemoryPercent   float64 `json:"memory_percent"`

	// Swap 指标
	SwapTotal   uint64  `json:"swap_total"`
	SwapUsed    uint64  `json:"swap_used"`
	SwapPercent float64 `json:"swap_percent"`
	SwapInRate  float64 `json:"swap_in_rate"`  // Swap 换入速率 (B/s)
	SwapOutRate float64 `json:"swap_out_rate"` // Swap 换出速率 (B/s)

	// 网络流量
	NetBytesRecv uint64  `json:"net_bytes_recv"` // 网络接收总字节
	NetBytesSent uint64  `json:"net_bytes_sent"` // 网络发送总字节
	NetRecvRate  float64 `json:"net_recv_rate"`  // 接收速率 (B/s)
	NetSendRate  float64 `json:"net_send_rate"`  // 发送速率 (B/s)

	// 磁盘 IO
	DiskReadRate  float64 `json:"disk_read_rate"`  // 磁盘读取速率 (B/s)
	DiskWriteRate float64 `json:"disk_write_rate"` // 磁盘写入速率 (B/s)
	DiskReadOps   float64 `json:"disk_read_ops"`   // 磁盘读取 IOPS
	DiskWriteOps  float64 `json:"disk_write_ops"`  // 磁盘写入 IOPS

	// 系统统计
	ProcessCount int `json:"process_count"` // 进程总数
	ThreadCount  int `json:"thread_count"`  // 线程总数
}

// ImpactEvent 影响事件
type ImpactEvent struct {
	Timestamp   time.Time     `json:"timestamp"`
	TargetPID   int32         `json:"target_pid"`   // 被影响的监控目标 PID
	TargetName  string        `json:"target_name"`  // 被影响的监控目标名称
	ImpactType  string        `json:"impact_type"`  // cpu/memory/disk_io/network/file/port
	Severity    string        `json:"severity"`     // low/medium/high/critical
	SourcePID   int32         `json:"source_pid"`   // 影响源进程 PID
	SourceName  string        `json:"source_name"`  // 影响源进程名
	Description string        `json:"description"`  // 影响描述
	Metrics     ImpactMetrics `json:"metrics"`      // 相关指标
	Suggestion  string        `json:"suggestion"`   // 处理建议
}

// ImpactMetrics 影响相关指标
type ImpactMetrics struct {
	SystemCPU    float64 `json:"system_cpu"`     // 系统 CPU 使用率
	SystemMemory float64 `json:"system_memory"`  // 系统内存使用率
	TargetCPU    float64 `json:"target_cpu"`     // 目标进程 CPU
	TargetMemory uint64  `json:"target_memory"`  // 目标进程内存
	SourceCPU    float64 `json:"source_cpu"`     // 影响源 CPU
	SourceMemory uint64  `json:"source_memory"`  // 影响源内存
	SourceDiskIO float64 `json:"source_disk_io"` // 影响源磁盘IO
	SourceNetIO  float64 `json:"source_net_io"`  // 影响源网络IO
	ConflictFile string  `json:"conflict_file,omitempty"` // 冲突文件路径
	ConflictPort int     `json:"conflict_port,omitempty"` // 冲突端口
}

// ImpactConfig 影响分析配置
type ImpactConfig struct {
	Enabled          bool    `json:"enabled"`           // 是否启用
	AnalysisInterval int     `json:"analysis_interval"` // 分析间隔（秒），默认5
	TopNProcesses    int     `json:"top_n_processes"`   // 分析 Top N 进程，默认10
	HistoryLen       int     `json:"history_len"`       // 影响记录保留数量，默认100

	// 系统级别阈值
	CPUThreshold     float64 `json:"cpu_threshold"`      // 系统 CPU 竞争阈值（%），默认80
	MemoryThreshold  float64 `json:"memory_threshold"`   // 系统内存压力阈值（%），默认85
	DiskIOThreshold  float64 `json:"disk_io_threshold"`  // 系统磁盘IO阈值（MB/s），默认100
	NetworkThreshold float64 `json:"network_threshold"` // 系统网络IO阈值（MB/s），默认100

	// 进程级别阈值（单个进程超过即触发检测）
	// 0 表示不检测该指标
	ProcCPUThreshold        float64 `json:"proc_cpu_threshold"`         // 进程 CPU 阈值（%），默认50
	ProcMemoryThreshold     float64 `json:"proc_memory_threshold"`      // 进程内存阈值（MB），默认1000
	ProcMemGrowthThreshold  float64 `json:"proc_mem_growth_threshold"`  // 进程内存增速阈值（MB/s），默认10
	ProcVMSThreshold        float64 `json:"proc_vms_threshold"`         // 进程虚拟内存阈值（MB），默认0（不检测）
	ProcFDsThreshold        int     `json:"proc_fds_threshold"`         // 进程句柄数阈值，默认1000
	ProcThreadsThreshold    int     `json:"proc_threads_threshold"`     // 进程线程数阈值，默认500
	ProcOpenFilesThreshold  int     `json:"proc_open_files_threshold"`  // 进程打开文件数阈值，默认500
	ProcDiskReadThreshold   float64 `json:"proc_disk_read_threshold"`   // 进程磁盘读阈值（MB/s），默认50
	ProcDiskWriteThreshold  float64 `json:"proc_disk_write_threshold"`  // 进程磁盘写阈值（MB/s），默认50
	ProcNetRecvThreshold    float64 `json:"proc_net_recv_threshold"`    // 进程网络收阈值（MB/s），默认50
	ProcNetSendThreshold    float64 `json:"proc_net_send_threshold"`    // 进程网络发阈值（MB/s），默认50

	// 资源冲突检测间隔
	FileCheckInterval int `json:"file_check_interval"` // 文件检测间隔（秒），默认30
	PortCheckInterval int `json:"port_check_interval"` // 端口检测间隔（秒），默认30

	// 兼容旧字段（已废弃，使用新字段）
	ProcessCPUThreshold     float64 `json:"process_cpu_threshold,omitempty"`
	ProcessMemoryThreshold  float64 `json:"process_memory_threshold,omitempty"`
	ProcessDiskIOThreshold  float64 `json:"process_disk_io_threshold,omitempty"`
	ProcessNetworkThreshold float64 `json:"process_network_threshold,omitempty"`
}
