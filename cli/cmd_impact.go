package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"monitor-agent/config"
)

// ImpactCommand 影响分析命令组
type ImpactCommand struct {
	cli *CLI
}

// NewImpactCommand 创建影响命令组
func NewImpactCommand(c *CLI) *ImpactCommand {
	return &ImpactCommand{cli: c}
}

// Handle 处理命令
func (cmd *ImpactCommand) Handle(subCmd string, args []string) {
	switch subCmd {
	case "list", "ls", "":
		cmd.listImpacts(args)
	case "summary", "sum":
		cmd.showSummary()
	case "config", "cfg":
		cmd.showConfig()
	case "set":
		cmd.setConfig(args)
	case "clear":
		cmd.clearImpacts()
	case "help", "h":
		cmd.PrintHelp()
	default:
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("未知子命令: %s", subCmd)))
		cmd.PrintHelp()
	}
}

// PrintHelp 打印帮助
func (cmd *ImpactCommand) PrintHelp() {
	fmt.Println(cmd.cli.formatter.Header("\n=== 影响分析命令 (impact) ==="))
	fmt.Println()
	fmt.Println("  list [n]              - 列出最近的影响事件 (默认20)")
	fmt.Println("  summary               - 显示影响统计汇总")
	fmt.Println("  config                - 显示影响分析配置")
	fmt.Println("  set <key> <value>     - 设置影响分析参数 (自动保存)")
	fmt.Println("  clear                 - 清除所有影响事件记录")
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("系统级阈值: cpu, memory, disk_io, network"))
	fmt.Println(cmd.cli.formatter.Info("进程级阈值: proc_cpu, proc_mem, proc_fds, proc_threads..."))
	fmt.Println(cmd.cli.formatter.Info("其他: enabled, interval"))
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("示例: impact set cpu 80"))
	fmt.Println(cmd.cli.formatter.Info("示例: impact set proc_mem 500"))
}

func (cmd *ImpactCommand) listImpacts(args []string) {
	count := 20
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			count = n
		}
	}

	impacts := cmd.cli.monitor.GetImpactEvents()
	if len(impacts) == 0 {
		fmt.Println(cmd.cli.formatter.Info("暂无影响事件"))
		return
	}

	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== 影响事件列表 (最近%d条) ===", count)))
	fmt.Println()

	// 表头
	headers := []string{"时间", "类型", "进程", "影响", "详情"}
	widths := []int{20, 10, 20, 10, 40}

	// 打印表头
	headerLine := ""
	for i, h := range headers {
		headerLine += fmt.Sprintf("%-*s", widths[i], h)
	}
	fmt.Println(cmd.cli.formatter.Bold(headerLine))
	fmt.Println(strings.Repeat("-", 100))

	// 倒序显示，最新的在前
	start := 0
	if len(impacts) > count {
		start = len(impacts) - count
	}

	for i := len(impacts) - 1; i >= start; i-- {
		imp := impacts[i]
		
		timeStr := imp.Timestamp.Format("01-02 15:04:05")
		typeStr := cmd.formatImpactType(imp.ImpactType)
		procStr := cmd.cli.formatter.Truncate(imp.SourceName, 18)
		levelStr := cmd.formatImpactLevel(imp.Severity)
		detailStr := cmd.cli.formatter.Truncate(imp.Description, 38)

		fmt.Printf("%-20s%-10s%-20s%-10s%-40s\n",
			timeStr, typeStr, procStr, levelStr, detailStr)
	}

	fmt.Println()
	fmt.Printf(cmd.cli.formatter.Info("共 %d 条影响事件"), len(impacts))
	fmt.Println()
}

func (cmd *ImpactCommand) formatImpactType(t string) string {
	switch strings.ToUpper(t) {
	case "CPU":
		return cmd.cli.formatter.Warning("CPU")
	case "MEMORY", "MEM":
		return cmd.cli.formatter.Error("内存")
	case "IO", "DISK":
		return cmd.cli.formatter.Info("IO")
	case "NETWORK", "NET":
		return cmd.cli.formatter.Header("网络")
	default:
		return t
	}
}

func (cmd *ImpactCommand) formatImpactLevel(level string) string {
	switch strings.ToLower(level) {
	case "critical", "严重":
		return cmd.cli.formatter.Error("严重")
	case "high", "高":
		return cmd.cli.formatter.Warning("高")
	case "medium", "中":
		return cmd.cli.formatter.Info("中")
	case "low", "低":
		return "低"
	default:
		return level
	}
}

func (cmd *ImpactCommand) showSummary() {
	impacts := cmd.cli.monitor.GetImpactEvents()
	
	fmt.Println(cmd.cli.formatter.Header("\n=== 影响分析统计 ==="))
	fmt.Println()

	if len(impacts) == 0 {
		fmt.Println(cmd.cli.formatter.Info("暂无影响事件"))
		return
	}

	// 统计
	typeCount := make(map[string]int)
	levelCount := make(map[string]int)
	processCount := make(map[string]int)
	var earliest, latest time.Time

	for i, imp := range impacts {
		typeCount[imp.ImpactType]++
		levelCount[imp.Severity]++
		processCount[imp.SourceName]++

		if i == 0 {
			earliest = imp.Timestamp
			latest = imp.Timestamp
		} else {
			if imp.Timestamp.Before(earliest) {
				earliest = imp.Timestamp
			}
			if imp.Timestamp.After(latest) {
				latest = imp.Timestamp
			}
		}
	}

	// 时间范围
	fmt.Printf("时间范围: %s - %s\n", earliest.Format("01-02 15:04"), latest.Format("01-02 15:04"))
	fmt.Printf("总事件数: %d\n", len(impacts))
	fmt.Println()

	// 按类型统计
	fmt.Println(cmd.cli.formatter.Bold("按类型统计:"))
	for t, c := range typeCount {
		pct := float64(c) / float64(len(impacts)) * 100
		bar := cmd.cli.formatter.ProgressBar(pct, 20)
		fmt.Printf("  %-10s %s %d (%.1f%%)\n", t, bar, c, pct)
	}
	fmt.Println()

	// 按级别统计
	fmt.Println(cmd.cli.formatter.Bold("按级别统计:"))
	for l, c := range levelCount {
		pct := float64(c) / float64(len(impacts)) * 100
		bar := cmd.cli.formatter.ProgressBar(pct, 20)
		fmt.Printf("  %-10s %s %d (%.1f%%)\n", l, bar, c, pct)
	}
	fmt.Println()

	// Top 5 影响进程
	fmt.Println(cmd.cli.formatter.Bold("Top 5 受影响进程:"))
	type procStat struct {
		name  string
		count int
	}
	var procs []procStat
	for name, count := range processCount {
		procs = append(procs, procStat{name, count})
	}
	// 简单排序
	for i := 0; i < len(procs)-1; i++ {
		for j := i + 1; j < len(procs); j++ {
			if procs[j].count > procs[i].count {
				procs[i], procs[j] = procs[j], procs[i]
			}
		}
	}
	for i := 0; i < len(procs) && i < 5; i++ {
		pct := float64(procs[i].count) / float64(len(impacts)) * 100
		fmt.Printf("  %d. %-20s %d次 (%.1f%%)\n", i+1, procs[i].name, procs[i].count, pct)
	}
}

func (cmd *ImpactCommand) showConfig() {
	cfg := cmd.cli.config.Impact

	fmt.Println(cmd.cli.formatter.Header("\n=== 影响分析配置 ==="))
	fmt.Println()

	fmt.Printf("  启用状态: %s\n", cmd.cli.formatter.FormatBool(cfg.Enabled))
	fmt.Println()
	
	fmt.Println(cmd.cli.formatter.Bold("系统级阈值:"))
	fmt.Printf("  CPU阈值:      %.0f%%\n", cfg.CPUThreshold)
	fmt.Printf("  内存阈值:     %.0f%%\n", cfg.MemoryThreshold)
	fmt.Printf("  磁盘IO阈值:   %.0f MB/s\n", cfg.DiskIOThreshold)
	fmt.Printf("  网络阈值:     %.0f MB/s\n", cfg.NetworkThreshold)
	fmt.Println()
	
	fmt.Println(cmd.cli.formatter.Bold("进程级阈值:"))
	fmt.Printf("  CPU:          %.0f%%\n", cfg.ProcCPUThreshold)
	fmt.Printf("  内存:         %.0f MB\n", cfg.ProcMemoryThreshold)
	fmt.Printf("  内存增速:     %.0f MB/s\n", cfg.ProcMemGrowthThreshold)
	fmt.Printf("  句柄数:       %d\n", cfg.ProcFDsThreshold)
	fmt.Printf("  线程数:       %d\n", cfg.ProcThreadsThreshold)
	fmt.Printf("  磁盘读:       %.0f MB/s\n", cfg.ProcDiskReadThreshold)
	fmt.Printf("  磁盘写:       %.0f MB/s\n", cfg.ProcDiskWriteThreshold)
	fmt.Printf("  网络收:       %.0f MB/s\n", cfg.ProcNetRecvThreshold)
	fmt.Printf("  网络发:       %.0f MB/s\n", cfg.ProcNetSendThreshold)
	fmt.Println()
	
	fmt.Println(cmd.cli.formatter.Bold("分析参数:"))
	fmt.Printf("  分析周期:     %d秒\n", cfg.AnalysisInterval)
	fmt.Printf("  最大记录:     %d\n", cfg.HistoryLen)
	fmt.Printf("  端口检测间隔: %d秒\n", cfg.PortCheckInterval)
	fmt.Printf("  文件检测间隔: %d秒\n", cfg.FileCheckInterval)
}

func (cmd *ImpactCommand) setConfig(args []string) {
	if len(args) < 2 {
		fmt.Println(cmd.cli.formatter.Error("用法: impact set <key> <value>"))
		fmt.Println()
		fmt.Println(cmd.cli.formatter.Info("系统级阈值:"))
		fmt.Println("  cpu, memory, disk_io, network")
		fmt.Println()
		fmt.Println(cmd.cli.formatter.Info("进程级阈值:"))
		fmt.Println("  proc_cpu, proc_mem, proc_mem_growth")
		fmt.Println("  proc_fds, proc_threads")
		fmt.Println("  proc_disk_read, proc_disk_write")
		fmt.Println("  proc_net_recv, proc_net_send")
		fmt.Println()
		fmt.Println(cmd.cli.formatter.Info("其他:"))
		fmt.Println("  enabled, interval")
		return
	}

	key := strings.ToLower(args[0])
	value := args[1]
	cfg := &cmd.cli.config.Impact

	var updated bool
	var msg string

	switch key {
	// 系统级阈值
	case "cpu", "cpu_threshold":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.CPUThreshold = v
			msg = fmt.Sprintf("系统CPU阈值: %.0f%%", v)
			updated = true
		}
	case "memory", "memory_threshold", "mem":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.MemoryThreshold = v
			msg = fmt.Sprintf("系统内存阈值: %.0f%%", v)
			updated = true
		}
	case "disk_io", "io_threshold", "io":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.DiskIOThreshold = v
			msg = fmt.Sprintf("系统磁盘IO阈值: %.0f MB/s", v)
			updated = true
		}
	case "network", "network_threshold", "net":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.NetworkThreshold = v
			msg = fmt.Sprintf("系统网络阈值: %.0f MB/s", v)
			updated = true
		}

	// 进程级阈值
	case "proc_cpu":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcCPUThreshold = v
			msg = fmt.Sprintf("进程CPU阈值: %.0f%%", v)
			updated = true
		}
	case "proc_mem", "proc_memory":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcMemoryThreshold = v
			msg = fmt.Sprintf("进程内存阈值: %.0f MB", v)
			updated = true
		}
	case "proc_mem_growth":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcMemGrowthThreshold = v
			msg = fmt.Sprintf("进程内存增速阈值: %.0f MB/s", v)
			updated = true
		}
	case "proc_fds":
		if v, err := strconv.Atoi(value); err == nil {
			cfg.ProcFDsThreshold = v
			msg = fmt.Sprintf("进程句柄数阈值: %d", v)
			updated = true
		}
	case "proc_threads":
		if v, err := strconv.Atoi(value); err == nil {
			cfg.ProcThreadsThreshold = v
			msg = fmt.Sprintf("进程线程数阈值: %d", v)
			updated = true
		}
	case "proc_disk_read":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcDiskReadThreshold = v
			msg = fmt.Sprintf("进程磁盘读阈值: %.0f MB/s", v)
			updated = true
		}
	case "proc_disk_write":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcDiskWriteThreshold = v
			msg = fmt.Sprintf("进程磁盘写阈值: %.0f MB/s", v)
			updated = true
		}
	case "proc_net_recv":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcNetRecvThreshold = v
			msg = fmt.Sprintf("进程网络收阈值: %.0f MB/s", v)
			updated = true
		}
	case "proc_net_send":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.ProcNetSendThreshold = v
			msg = fmt.Sprintf("进程网络发阈值: %.0f MB/s", v)
			updated = true
		}

	// 其他配置
	case "enabled":
		if v, err := strconv.ParseBool(value); err == nil {
			cfg.Enabled = v
			if v {
				msg = "影响分析已启用"
			} else {
				msg = "影响分析已禁用"
			}
			updated = true
		}
	case "interval", "analysis_interval":
		if v, err := strconv.Atoi(value); err == nil && v > 0 {
			cfg.AnalysisInterval = v
			msg = fmt.Sprintf("分析间隔: %d秒", v)
			updated = true
		}

	default:
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("未知配置项: %s", key)))
		return
	}

	if !updated {
		fmt.Println(cmd.cli.formatter.Error("无效的数值"))
		return
	}

	// 同步到 ImpactAnalyzer
	analyzer := cmd.cli.monitor.GetImpactAnalyzer()
	if analyzer != nil {
		analyzer.UpdateConfig(*cfg)
	}

	// 保存到配置文件
	if cmd.cli.configFile != "" {
		if err := config.SaveConfig(cmd.cli.configFile, cmd.cli.config); err != nil {
			fmt.Println(cmd.cli.formatter.Warning(fmt.Sprintf("保存配置失败: %v", err)))
		}
	}

	fmt.Println(cmd.cli.formatter.Success(msg + " (已保存)"))
}

func (cmd *ImpactCommand) clearImpacts() {
	fmt.Print("确认清除所有影响事件? (y/n): ")
	if cmd.cli.scanner.Scan() {
		input := strings.ToLower(strings.TrimSpace(cmd.cli.scanner.Text()))
		if input == "y" || input == "yes" {
			cmd.cli.monitor.ClearImpactEvents()
			fmt.Println(cmd.cli.formatter.Success("所有影响事件已清除"))
		} else {
			fmt.Println(cmd.cli.formatter.Info("操作已取消"))
		}
	}
}
