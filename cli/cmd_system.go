package cli

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
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
		cmd.showStatus()
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
	fmt.Println("  status                - 显示系统整体状态")
	fmt.Println("  top [n]               - 显示Top N进程 (默认10)")
	fmt.Println("  ps [pattern]          - 列出进程 (可按名称过滤)")
	fmt.Println("  events [n]            - 显示最近事件 (默认20)")
	fmt.Println("  watch <pid>           - 实时监控指定进程")
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("示例:"))
	fmt.Println("  system top 20         - 显示CPU占用最高的20个进程")
	fmt.Println("  system ps java        - 列出名称包含java的进程")
	fmt.Println("  system watch 1234     - 实时监控PID为1234的进程")
}

func (cmd *SystemCommand) showStatus() {
	fmt.Println(cmd.cli.formatter.Header("\n=== 系统状态 ==="))
	fmt.Println()

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
	if cpuPercent, err := cpu.Percent(time.Second, false); err == nil && len(cpuPercent) > 0 {
		pct := cpuPercent[0]
		bar := cmd.cli.formatter.ProgressBar(pct, 30)
		fmt.Printf("  使用率:     %s %s\n", bar, cmd.cli.formatter.FormatPercent(pct))
	}
	fmt.Println()

	// 内存信息
	fmt.Println(cmd.cli.formatter.Bold("内存:"))
	if memInfo, err := mem.VirtualMemory(); err == nil {
		bar := cmd.cli.formatter.ProgressBar(memInfo.UsedPercent, 30)
		fmt.Printf("  总量:       %s\n", cmd.cli.formatter.FormatBytes(memInfo.Total))
		fmt.Printf("  已用:       %s\n", cmd.cli.formatter.FormatBytes(memInfo.Used))
		fmt.Printf("  可用:       %s\n", cmd.cli.formatter.FormatBytes(memInfo.Available))
		fmt.Printf("  使用率:     %s %s\n", bar, cmd.cli.formatter.FormatPercent(memInfo.UsedPercent))
	}
	fmt.Println()

	// 磁盘信息
	fmt.Println(cmd.cli.formatter.Bold("磁盘:"))
	if partitions, err := disk.Partitions(false); err == nil {
		for _, p := range partitions {
			if usage, err := disk.Usage(p.Mountpoint); err == nil {
				bar := cmd.cli.formatter.ProgressBar(usage.UsedPercent, 20)
				fmt.Printf("  %-10s %s %s / %s (%s)\n",
					p.Mountpoint,
					bar,
					cmd.cli.formatter.FormatBytes(usage.Used),
					cmd.cli.formatter.FormatBytes(usage.Total),
					cmd.cli.formatter.FormatPercent(usage.UsedPercent))
			}
		}
	}
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
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			count = n
		}
	}

	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== Top %d 进程 (按CPU排序) ===", count)))
	fmt.Println()

	procs, err := process.Processes()
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
		return
	}

	type procInfo struct {
		pid     int32
		name    string
		cpu     float64
		mem     float32
		threads int32
	}

	var procList []procInfo
	for _, p := range procs {
		name, _ := p.Name()
		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryPercent()
		threads, _ := p.NumThreads()

		procList = append(procList, procInfo{
			pid:     p.Pid,
			name:    name,
			cpu:     cpu,
			mem:     mem,
			threads: threads,
		})
	}

	// 按CPU排序
	for i := 0; i < len(procList)-1; i++ {
		for j := i + 1; j < len(procList); j++ {
			if procList[j].cpu > procList[i].cpu {
				procList[i], procList[j] = procList[j], procList[i]
			}
		}
	}

	// 表头
	fmt.Println(cmd.cli.formatter.Bold(fmt.Sprintf("%-8s %-25s %10s %10s %8s", "PID", "名称", "CPU%", "内存%", "线程")))
	fmt.Println(strings.Repeat("-", 65))

	for i := 0; i < len(procList) && i < count; i++ {
		p := procList[i]
		name := cmd.cli.formatter.Truncate(p.name, 23)
		cpuStr := fmt.Sprintf("%.1f", p.cpu)
		memStr := fmt.Sprintf("%.1f", p.mem)

		// 高CPU高亮
		if p.cpu > 50 {
			cpuStr = cmd.cli.formatter.Error(cpuStr)
		} else if p.cpu > 20 {
			cpuStr = cmd.cli.formatter.Warning(cpuStr)
		}

		fmt.Printf("%-8d %-25s %10s %10s %8d\n", p.pid, name, cpuStr, memStr, p.threads)
	}
}

func (cmd *SystemCommand) listProcesses(args []string) {
	pattern := ""
	if len(args) > 0 {
		pattern = strings.ToLower(args[0])
	}

	fmt.Println(cmd.cli.formatter.Header("\n=== 进程列表 ==="))
	fmt.Println()

	procs, err := process.Processes()
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
		return
	}

	fmt.Println(cmd.cli.formatter.Bold(fmt.Sprintf("%-8s %-30s %10s %10s %-20s", "PID", "名称", "CPU%", "内存%", "状态")))
	fmt.Println(strings.Repeat("-", 85))

	count := 0
	for _, p := range procs {
		name, _ := p.Name()
		if pattern != "" && !strings.Contains(strings.ToLower(name), pattern) {
			continue
		}

		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryPercent()
		status, _ := p.Status()

		statusStr := strings.Join(status, ",")
		name = cmd.cli.formatter.Truncate(name, 28)

		fmt.Printf("%-8d %-30s %10.1f %10.1f %-20s\n", p.Pid, name, cpu, mem, statusStr)
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
