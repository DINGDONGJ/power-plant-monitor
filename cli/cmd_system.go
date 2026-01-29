package cli

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"monitor-agent/types"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemCommand 系统信息命令组
type SystemCommand struct {
	cli *CLI
}

// NewSystemCommand 创建系统命令组
func NewSystemCommand(c *CLI) *SystemCommand {
	return &SystemCommand{cli: c}
}

// Handle 处理命令
func (cmd *SystemCommand) Handle(subCmd string, args []string) {
	switch subCmd {
	case "status", "stat", "":
		cmd.showStatus(args)
	case "top":
		cmd.showTopProcesses(args)
	case "ps":
		cmd.listProcesses(args)
	case "events", "ev":
		cmd.showEvents(args)
	case "watch":
		cmd.watchProcess(args)
	case "help", "h":
		cmd.PrintHelp()
	default:
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("未知子命令: %s", subCmd)))
		cmd.PrintHelp()
	}
}

// PrintHelp 打印帮助
func (cmd *SystemCommand) PrintHelp() {
	fmt.Println(cmd.cli.formatter.Header("\n=== 系统信息命令 (system) ==="))
	fmt.Println()
	fmt.Println("  status [-1]           - 显示系统状态 (默认动态刷新, -1 只显示一次)")
	fmt.Println("  top [n] [-1]          - 显示Top N进程 (默认动态刷新, -1 只显示一次)")
	fmt.Println("  ps [pattern]          - 列出进程 (可按名称过滤)")
	fmt.Println("  events [n]            - 显示最近事件 (默认20)")
	fmt.Println("  watch <pid>           - 实时监控指定进程")
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("示例:"))
	fmt.Println("  system top 20         - 动态刷新显示Top 20进程")
	fmt.Println("  system top 10 -1      - 只显示一次Top 10进程")
	fmt.Println("  system ps java        - 列出名称包含java的进程")
	fmt.Println("  system watch 1234     - 实时监控PID为1234的进程")
}

func (cmd *SystemCommand) showStatus(args []string) {
	// 检查是否只显示一次
	onceMode := false
	for _, arg := range args {
		if arg == "-1" || arg == "once" {
			onceMode = true
		}
	}

	if onceMode {
		cmd.renderStatus()
		return
	}

	// 默认动态刷新
	fmt.Println(cmd.cli.formatter.Info("动态监控模式，按 Enter 键退出..."))

	stopChan := make(chan struct{})
	go func() {
		cmd.cli.scanner.Scan()
		close(stopChan)
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	cmd.renderStatusWatch()

	for {
		select {
		case <-stopChan:
			fmt.Println(cmd.cli.formatter.Info("\n已退出动态监控"))
			return
		case <-ticker.C:
			cmd.renderStatusWatch()
		}
	}
}

func (cmd *SystemCommand) renderStatusWatch() {
	fmt.Print("\033[H\033[J")
	now := time.Now().Format("15:04:05")
	fmt.Printf("=== 系统状态 === [%s] 按 Enter 退出\n\n", now)
	cmd.renderStatusContent()
}

func (cmd *SystemCommand) renderStatus() {
	fmt.Println(cmd.cli.formatter.Header("\n=== 系统状态 ==="))
	fmt.Println()
	cmd.renderStatusContent()
}

func (cmd *SystemCommand) renderStatusContent() {
	// 使用 monitor.GetSystemMetrics()，与 Web 数据源一致
	sysMetrics, err := cmd.cli.monitor.GetSystemMetrics()
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("获取系统指标失败: %v", err)))
		return
	}

	// 主机信息
	if info, err := host.Info(); err == nil {
		fmt.Println(cmd.cli.formatter.Bold("主机信息:"))
		fmt.Printf("  主机名:     %s\n", info.Hostname)
		fmt.Printf("  操作系统:   %s %s\n", info.Platform, info.PlatformVersion)
		fmt.Printf("  内核版本:   %s\n", info.KernelVersion)
		uptime := time.Duration(info.Uptime) * time.Second
		fmt.Printf("  运行时间:   %s\n", cmd.formatUptime(uptime))
		fmt.Println()
	}

	// CPU信息
	fmt.Println(cmd.cli.formatter.Bold("CPU:"))
	fmt.Printf("  逻辑核心:   %d\n", runtime.NumCPU())
	bar := cmd.cli.formatter.ProgressBar(sysMetrics.CPUPercent, 30)
	fmt.Printf("  总使用率:   %s %s\n", bar, cmd.cli.formatter.FormatPercent(sysMetrics.CPUPercent))
	fmt.Printf("  用户态:     %.1f%%    内核态: %.1f%%    IO等待: %.1f%%    空闲: %.1f%%\n",
		sysMetrics.CPUUser, sysMetrics.CPUSystem, sysMetrics.CPUIowait, sysMetrics.CPUIdle)
	if sysMetrics.LoadAvg1 > 0 || sysMetrics.LoadAvg5 > 0 || sysMetrics.LoadAvg15 > 0 {
		fmt.Printf("  系统负载:   %.2f / %.2f / %.2f (1/5/15分钟)\n",
			sysMetrics.LoadAvg1, sysMetrics.LoadAvg5, sysMetrics.LoadAvg15)
	}
	fmt.Println()

	// 内存信息
	fmt.Println(cmd.cli.formatter.Bold("内存:"))
	memBar := cmd.cli.formatter.ProgressBar(sysMetrics.MemoryPercent, 30)
	fmt.Printf("  总量:       %s\n", FormatBytes(sysMetrics.MemoryTotal))
	fmt.Printf("  已用:       %s\n", FormatBytes(sysMetrics.MemoryUsed))
	fmt.Printf("  可用:       %s\n", FormatBytes(sysMetrics.MemoryAvailable))
	fmt.Printf("  使用率:     %s %s\n", memBar, cmd.cli.formatter.FormatPercent(sysMetrics.MemoryPercent))
	fmt.Println()

	// Swap信息
	if sysMetrics.SwapTotal > 0 {
		fmt.Println(cmd.cli.formatter.Bold("Swap:"))
		swapBar := cmd.cli.formatter.ProgressBar(sysMetrics.SwapPercent, 30)
		fmt.Printf("  总量:       %s\n", FormatBytes(sysMetrics.SwapTotal))
		fmt.Printf("  已用:       %s\n", FormatBytes(sysMetrics.SwapUsed))
		fmt.Printf("  使用率:     %s %s\n", swapBar, cmd.cli.formatter.FormatPercent(sysMetrics.SwapPercent))
		if sysMetrics.SwapInRate > 0 || sysMetrics.SwapOutRate > 0 {
			fmt.Printf("  换入/换出:  %s/s / %s/s\n",
				FormatBytes(uint64(sysMetrics.SwapInRate)), FormatBytes(uint64(sysMetrics.SwapOutRate)))
		}
		fmt.Println()
	}

	// 网络流量
	fmt.Println(cmd.cli.formatter.Bold("网络流量:"))
	fmt.Printf("  接收速率:   %s/s\n", FormatBytes(uint64(sysMetrics.NetRecvRate)))
	fmt.Printf("  发送速率:   %s/s\n", FormatBytes(uint64(sysMetrics.NetSendRate)))
	fmt.Printf("  累计接收:   %s\n", FormatBytes(sysMetrics.NetBytesRecv))
	fmt.Printf("  累计发送:   %s\n", FormatBytes(sysMetrics.NetBytesSent))
	fmt.Println()

	// 磁盘IO
	fmt.Println(cmd.cli.formatter.Bold("磁盘IO:"))
	fmt.Printf("  读取速率:   %s/s    IOPS: %.0f\n", FormatBytes(uint64(sysMetrics.DiskReadRate)), sysMetrics.DiskReadOps)
	fmt.Printf("  写入速率:   %s/s    IOPS: %.0f\n", FormatBytes(uint64(sysMetrics.DiskWriteRate)), sysMetrics.DiskWriteOps)
	fmt.Println()

	// 磁盘空间
	fmt.Println(cmd.cli.formatter.Bold("磁盘空间:"))
	if partitions, err := disk.Partitions(false); err == nil {
		for _, p := range partitions {
			if usage, err := disk.Usage(p.Mountpoint); err == nil {
				diskBar := cmd.cli.formatter.ProgressBar(usage.UsedPercent, 20)
				fmt.Printf("  %-10s %s %s / %s (%s)\n",
					p.Mountpoint,
					diskBar,
					cmd.cli.formatter.FormatBytes(usage.Used),
					cmd.cli.formatter.FormatBytes(usage.Total),
					cmd.cli.formatter.FormatPercent(usage.UsedPercent))
			}
		}
	}
	fmt.Println()

	// 进程统计
	fmt.Println(cmd.cli.formatter.Bold("进程统计:"))
	fmt.Printf("  进程总数:   %d\n", sysMetrics.ProcessCount)
	fmt.Printf("  线程总数:   %d\n", sysMetrics.ThreadCount)
	fmt.Println()

	// 监控状态
	fmt.Println(cmd.cli.formatter.Bold("监控状态:"))
	targets := cmd.cli.monitor.GetTargets()
	fmt.Printf("  监控目标:   %d\n", len(targets))
	events := cmd.cli.monitor.GetEvents()
	fmt.Printf("  事件总数:   %d\n", len(events))
	impacts := cmd.cli.monitor.GetImpactEvents()
	fmt.Printf("  影响事件:   %d\n", len(impacts))
}

func (cmd *SystemCommand) formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时 %d分钟", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

func (cmd *SystemCommand) showTopProcesses(args []string) {
	count := 10
	onceMode := false

	// 解析参数
	for _, arg := range args {
		if arg == "-1" || arg == "once" || arg == "-once" {
			onceMode = true
		} else if n, err := strconv.Atoi(arg); err == nil && n > 0 {
			count = n
		}
	}

	if onceMode {
		cmd.showTopProcessesOnce(count)
		return
	}

	// 默认动态刷新
	cmd.showTopProcessesWatch(count)
}

func (cmd *SystemCommand) showTopProcessesOnce(count int) {
	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== Top %d 进程 (按CPU排序) ===", count)))
	fmt.Println()

	procList := cmd.getTopProcessList()
	if procList == nil {
		return
	}

	cmd.printProcessTable(procList, count)
}

func (cmd *SystemCommand) showTopProcessesWatch(count int) {
	fmt.Println(cmd.cli.formatter.Info("动态监控模式，按 Enter 键退出..."))
	fmt.Println()

	// 创建一个 channel 来接收退出信号
	stopChan := make(chan struct{})

	// 在后台监听用户输入
	go func() {
		cmd.cli.scanner.Scan()
		close(stopChan)
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// 先显示一次
	cmd.renderTopProcesses(count)

	for {
		select {
		case <-stopChan:
			fmt.Println(cmd.cli.formatter.Info("\n已退出动态监控"))
			return
		case <-ticker.C:
			cmd.renderTopProcesses(count)
		}
	}
}

func (cmd *SystemCommand) renderTopProcesses(count int) {
	fmt.Print("\033[H\033[J")
	now := time.Now().Format("15:04:05")
	fmt.Printf("=== Top %d 进程 (按CPU排序) === [%s] 按 Enter 退出\n\n", count, now)

	procList := cmd.getTopProcessList()
	if procList == nil {
		return
	}

	cmd.printProcessTable(procList, count)
}

func (cmd *SystemCommand) printProcessTable(procList []types.ProcessInfo, count int) {
	// 表头：与 Web 页面保持一致
	fmt.Printf("%-7s %-18s %6s %9s %9s %8s %8s %8s %8s %6s %s\n",
		"PID", "名称", "CPU%", "内存", "内存增速", "磁盘读", "磁盘写", "网络收", "网络发", "线程", "用户")
	fmt.Println(strings.Repeat("-", 120))

	for i := 0; i < len(procList) && i < count; i++ {
		p := procList[i]
		name := cmd.cli.formatter.Truncate(p.Name, 16)
		user := cmd.cli.formatter.Truncate(p.Username, 12)

		// CPU 高亮
		cpuStr := fmt.Sprintf("%6.1f", p.CPUPct)
		if p.CPUPct > 50 {
			cpuStr = cmd.cli.formatter.Error(cpuStr)
		} else if p.CPUPct > 20 {
			cpuStr = cmd.cli.formatter.Warning(cpuStr)
		}

		fmt.Printf("%-7d %-18s %s %9s %9s %8s %8s %8s %8s %6d %s\n",
			p.PID,
			name,
			cpuStr,
			FormatBytes(p.RSSBytes),
			FormatMemGrowth(p.RSSGrowthRate),
			FormatBytesRate(p.DiskReadRate),
			FormatBytesRate(p.DiskWriteRate),
			FormatBytesRate(p.NetRecvRate),
			FormatBytesRate(p.NetSendRate),
			p.NumThreads,
			user,
		)
	}
}

func (cmd *SystemCommand) getTopProcessList() []types.ProcessInfo {
	procs, err := cmd.cli.monitor.ListAllProcesses()
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
		return nil
	}

	// 按CPU排序
	for i := 0; i < len(procs)-1; i++ {
		for j := i + 1; j < len(procs); j++ {
			if procs[j].CPUPct > procs[i].CPUPct {
				procs[i], procs[j] = procs[j], procs[i]
			}
		}
	}

	return procs
}

func (cmd *SystemCommand) listProcesses(args []string) {
	pattern := ""
	if len(args) > 0 {
		pattern = strings.ToLower(args[0])
	}

	fmt.Println(cmd.cli.formatter.Header("\n=== 进程列表 ==="))
	fmt.Println()

	// 使用 monitor 的 ListAllProcesses，与 Web 数据源一致
	procs, err := cmd.cli.monitor.ListAllProcesses()
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
		return
	}

	// 获取总内存用于计算百分比
	var totalMem uint64
	if memInfo, _ := mem.VirtualMemory(); memInfo != nil {
		totalMem = memInfo.Total
	}

	fmt.Println(cmd.cli.formatter.Bold(fmt.Sprintf("%-8s %-30s %10s %10s %-20s", "PID", "名称", "CPU%", "内存%", "状态")))
	fmt.Println(strings.Repeat("-", 85))

	count := 0
	for _, p := range procs {
		if pattern != "" && !strings.Contains(strings.ToLower(p.Name), pattern) {
			continue
		}

		var memPct float64
		if totalMem > 0 {
			memPct = float64(p.RSSBytes) / float64(totalMem) * 100
		}

		name := cmd.cli.formatter.Truncate(p.Name, 28)

		fmt.Printf("%-8d %-30s %10.1f %10.1f %-20s\n", p.PID, name, p.CPUPct, memPct, p.Status)
		count++

		if count >= 100 {
			fmt.Println(cmd.cli.formatter.Info("... 仅显示前100条，请使用过滤条件缩小范围"))
			break
		}
	}

	fmt.Println()
	if pattern != "" {
		fmt.Printf(cmd.cli.formatter.Info("匹配 '%s' 的进程: %d\n"), pattern, count)
	} else {
		fmt.Printf(cmd.cli.formatter.Info("总进程数: %d\n"), len(procs))
	}
}

func (cmd *SystemCommand) showEvents(args []string) {
	count := 20
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			count = n
		}
	}

	events := cmd.cli.monitor.GetEvents()
	if len(events) == 0 {
		fmt.Println(cmd.cli.formatter.Info("暂无事件记录"))
		return
	}

	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== 最近事件 (最多%d条) ===", count)))
	fmt.Println()

	fmt.Println(cmd.cli.formatter.Bold(fmt.Sprintf("%-20s %-10s %-10s %-40s", "时间", "类型", "PID", "描述")))
	fmt.Println(strings.Repeat("-", 85))

	start := 0
	if len(events) > count {
		start = len(events) - count
	}

	for i := len(events) - 1; i >= start; i-- {
		ev := events[i]
		timeStr := ev.Timestamp.Format("01-02 15:04:05")
		typeStr := cmd.formatEventType(ev.Type)
		desc := cmd.cli.formatter.Truncate(ev.Message, 38)

		fmt.Printf("%-20s %-10s %-10d %-40s\n", timeStr, typeStr, ev.PID, desc)
	}

	fmt.Println()
	fmt.Printf(cmd.cli.formatter.Info("共 %d 条事件\n"), len(events))
}

func (cmd *SystemCommand) formatEventType(t string) string {
	switch strings.ToUpper(t) {
	case "START":
		return cmd.cli.formatter.Success("启动")
	case "STOP":
		return cmd.cli.formatter.Error("停止")
	case "RESTART":
		return cmd.cli.formatter.Warning("重启")
	case "ALERT":
		return cmd.cli.formatter.Error("告警")
	case "INFO":
		return cmd.cli.formatter.Info("信息")
	default:
		return t
	}
}

func (cmd *SystemCommand) watchProcess(args []string) {
	if len(args) == 0 {
		fmt.Println(cmd.cli.formatter.Error("用法: system watch <pid>"))
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error("无效的PID"))
		return
	}

	p, err := process.NewProcess(int32(pid))
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("进程不存在: %d", pid)))
		return
	}

	name, _ := p.Name()
	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== 实时监控: %s (PID: %d) ===", name, pid)))
	fmt.Println(cmd.cli.formatter.Info("按 Ctrl+C 退出监控"))
	fmt.Println()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// 清除上一行的ANSI序列
	clearLine := "\033[2K\r"

	for i := 0; i < 30; i++ { // 监控60秒
		select {
		case <-ticker.C:
			// 检查进程是否存在
			if running, _ := p.IsRunning(); !running {
				fmt.Println(cmd.cli.formatter.Error("\n进程已退出"))
				return
			}

			cpu, _ := p.CPUPercent()
			mem, _ := p.MemoryPercent()
			memInfo, _ := p.MemoryInfo()
			threads, _ := p.NumThreads()
			conns, _ := p.Connections()

			fmt.Print(clearLine)
			fmt.Printf("CPU: %-6.1f%% | 内存: %-6.1f%% (%s) | 线程: %-4d | 连接: %-3d",
				cpu, mem,
				cmd.cli.formatter.FormatBytes(memInfo.RSS),
				threads, len(conns))

			// 检查是否有输入（简单的退出检测）
			if i == 29 {
				fmt.Println("\n" + cmd.cli.formatter.Info("监控超时，自动退出"))
			}
		}
	}
}

func (cmd *SystemCommand) findProcess(nameOrPid string) *process.Process {
	// 尝试作为PID
	if pid, err := strconv.ParseInt(nameOrPid, 10, 32); err == nil {
		if p, err := process.NewProcess(int32(pid)); err == nil {
			return p
		}
	}

	// 作为名称搜索
	procs, _ := process.Processes()
	for _, p := range procs {
		if name, _ := p.Name(); strings.EqualFold(name, nameOrPid) {
			return p
		}
	}
	return nil
}

// GetHostname 获取主机名
func GetHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}
