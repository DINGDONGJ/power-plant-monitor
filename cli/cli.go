package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"monitor-agent/monitor"
	"monitor-agent/types"
)

// CLI 命令行交互界面
type CLI struct {
	monitor *monitor.MultiMonitor
	scanner *bufio.Scanner
	running bool
}

// NewCLI 创建命令行界面
func NewCLI(m *monitor.MultiMonitor) *CLI {
	return &CLI{
		monitor: m,
		scanner: bufio.NewScanner(os.Stdin),
		running: true,
	}
}

// Run 运行命令行交互
func (c *CLI) Run() {
	c.printBanner()
	c.printHelp()

	for c.running {
		fmt.Print("\n> ")
		if !c.scanner.Scan() {
			break
		}

		line := strings.TrimSpace(c.scanner.Text())
		if line == "" {
			continue
		}

		c.handleCommand(line)
	}
}

func (c *CLI) printBanner() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║     电厂核心软件监视保障系统 - 命令行模式                  ║")
	fmt.Println("║     Monitor Agent CLI v1.0                                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
}

func (c *CLI) printHelp() {
	fmt.Println("\n可用命令:")
	fmt.Println("  add <pid|name> [alias]  - 添加监控目标")
	fmt.Println("  remove <pid>            - 移除监控目标")
	fmt.Println("  list                    - 列出所有监控目标")
	fmt.Println("  status                  - 显示系统状态")
	fmt.Println("  top [n]                 - 显示 Top N 进程 (默认 10)")
	fmt.Println("  events [n]              - 显示最近 N 条事件 (默认 20)")
	fmt.Println("  changes [n]             - 显示最近 N 条进程变化 (默认 20)")
	fmt.Println("  watch <pid>             - 实时监控指定进程")
	fmt.Println("  help                    - 显示帮助信息")
	fmt.Println("  exit                    - 退出程序")
}

func (c *CLI) handleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "add":
		c.cmdAdd(args)
	case "remove", "rm":
		c.cmdRemove(args)
	case "list", "ls":
		c.cmdList()
	case "status", "stat":
		c.cmdStatus()
	case "top":
		c.cmdTop(args)
	case "events":
		c.cmdEvents(args)
	case "changes":
		c.cmdChanges(args)
	case "watch":
		c.cmdWatch(args)
	case "help", "h", "?":
		c.printHelp()
	case "exit", "quit", "q":
		c.running = false
		fmt.Println("再见！")
	default:
		fmt.Printf("未知命令: %s (输入 'help' 查看帮助)\n", cmd)
	}
}

func (c *CLI) cmdAdd(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: add <pid|name> [alias]")
		return
	}

	var target types.MonitorTarget
	var alias string
	if len(args) > 1 {
		alias = strings.Join(args[1:], " ")
	}

	// 尝试解析为 PID
	if pid, err := strconv.ParseInt(args[0], 10, 32); err == nil {
		// 验证进程是否存在
		processes, err := c.monitor.ListAllProcesses()
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}

		var found *types.ProcessInfo
		for i := range processes {
			if processes[i].PID == int32(pid) {
				found = &processes[i]
				break
			}
		}

		if found == nil {
			fmt.Printf("错误: 进程 PID %d 不存在\n", pid)
			return
		}

		target = types.MonitorTarget{
			PID:     int32(pid),
			Name:    found.Name,
			Alias:   alias,
			Cmdline: found.Cmdline,
		}
	} else {
		// 按进程名查找
		processes, err := c.monitor.ListAllProcesses()
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}

		var matches []types.ProcessInfo
		for i := range processes {
			if strings.Contains(strings.ToLower(processes[i].Name), strings.ToLower(args[0])) {
				matches = append(matches, processes[i])
			}
		}

		if len(matches) == 0 {
			fmt.Printf("错误: 未找到名称包含 '%s' 的进程\n", args[0])
			return
		}

		if len(matches) > 1 {
			fmt.Printf("找到 %d 个匹配的进程:\n", len(matches))
			for _, p := range matches {
				fmt.Printf("  PID %d: %s\n", p.PID, p.Name)
			}
			fmt.Println("请使用 PID 精确指定")
			return
		}

		target = types.MonitorTarget{
			PID:     matches[0].PID,
			Name:    matches[0].Name,
			Alias:   alias,
			Cmdline: matches[0].Cmdline,
		}
	}

	if err := c.monitor.AddTarget(target); err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	displayName := target.Name
	if target.Alias != "" {
		displayName = fmt.Sprintf("%s (%s)", target.Alias, target.Name)
	}
	fmt.Printf("✓ 已添加监控目标: %s [PID %d]\n", displayName, target.PID)
}

func (c *CLI) cmdRemove(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: remove <pid>")
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Printf("错误: 无效的 PID '%s'\n", args[0])
		return
	}

	c.monitor.RemoveTarget(int32(pid))
	fmt.Printf("✓ 已移除监控目标 PID %d\n", pid)
}

func (c *CLI) cmdList() {
	targets := c.monitor.GetTargets()
	if len(targets) == 0 {
		fmt.Println("当前没有监控目标")
		return
	}

	// 获取所有进程信息（包含完整指标）
	allProcesses, err := c.monitor.ListAllProcesses()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	// 构建 PID 到进程信息的映射
	processMap := make(map[int32]*types.ProcessInfo)
	for i := range allProcesses {
		processMap[allProcesses[i].PID] = &allProcesses[i]
	}

	fmt.Printf("\n监控目标列表 (%d 个):\n", len(targets))
	fmt.Println(strings.Repeat("─", 120))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PID\t名称\t别名\t状态\tCPU%\t内存\t内存增速\t磁盘读\t磁盘写\t网络收\t网络发")
	fmt.Fprintln(w, strings.Repeat("─", 120))

	for _, t := range targets {
		p, exists := processMap[t.PID]
		
		status := "停止"
		cpu := "-"
		mem := "-"
		memGrowth := "-"
		diskRead := "-"
		diskWrite := "-"
		netRecv := "-"
		netSend := "-"

		if exists {
			status = "运行"
			cpu = fmt.Sprintf("%.1f", p.CPUPct)
			mem = formatBytes(p.RSSBytes)
			memGrowth = formatMemGrowth(p.RSSGrowthRate)
			diskRead = formatBytes(uint64(p.DiskReadRate)) + "/s"
			diskWrite = formatBytes(uint64(p.DiskWriteRate)) + "/s"
			netRecv = formatBytes(uint64(p.NetRecvRate)) + "/s"
			netSend = formatBytes(uint64(p.NetSendRate)) + "/s"
		}

		alias := t.Alias
		if alias == "" {
			alias = "-"
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			t.PID, truncate(t.Name, 15), truncate(alias, 10), status, 
			cpu, mem, memGrowth, diskRead, diskWrite, netRecv, netSend)
	}

	w.Flush()
	fmt.Println(strings.Repeat("─", 120))
}

func (c *CLI) cmdStatus() {
	sysMetrics, err := c.monitor.GetSystemMetrics()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Println("\n系统状态:")
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("CPU 使用率:    %.1f%% (User: %.1f%%, System: %.1f%%, IOWait: %.1f%%)\n",
		sysMetrics.CPUPercent, sysMetrics.CPUUser, sysMetrics.CPUSystem, sysMetrics.CPUIowait)
	fmt.Printf("内存使用率:    %.1f%% (%s / %s, 可用: %s)\n",
		sysMetrics.MemoryPercent,
		formatBytes(sysMetrics.MemoryUsed),
		formatBytes(sysMetrics.MemoryTotal),
		formatBytes(sysMetrics.MemoryAvailable))
	fmt.Printf("Swap 使用率:   %.1f%% (%s / %s)\n",
		sysMetrics.SwapPercent,
		formatBytes(sysMetrics.SwapUsed),
		formatBytes(sysMetrics.SwapTotal))
	fmt.Printf("网络流量:      ↓ %s/s  ↑ %s/s\n",
		formatBytes(uint64(sysMetrics.NetRecvRate)),
		formatBytes(uint64(sysMetrics.NetSendRate)))
	fmt.Printf("磁盘 IO:       读 %s/s  写 %s/s\n",
		formatBytes(uint64(sysMetrics.DiskReadRate)),
		formatBytes(uint64(sysMetrics.DiskWriteRate)))
	fmt.Println(strings.Repeat("─", 60))

	targets := c.monitor.GetTargets()
	fmt.Printf("\n监控目标: %d 个\n", len(targets))
	fmt.Printf("监控状态: %s\n", map[bool]string{true: "运行中", false: "已停止"}[c.monitor.IsRunning()])
}

func (c *CLI) cmdTop(args []string) {
	n := 10
	if len(args) > 0 {
		if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
			n = num
		}
	}

	processes, err := c.monitor.ListAllProcesses()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	// 按 CPU 排序
	sortByCPU(processes)

	if len(processes) > n {
		processes = processes[:n]
	}

	fmt.Printf("\nTop %d 进程 (按 CPU 排序):\n", n)
	fmt.Println(strings.Repeat("─", 100))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PID\t名称\tCPU%\t内存\t磁盘读\t磁盘写\t网络收\t网络发")
	fmt.Fprintln(w, strings.Repeat("─", 100))

	for _, p := range processes {
		fmt.Fprintf(w, "%d\t%s\t%.1f\t%s\t%s/s\t%s/s\t%s/s\t%s/s\n",
			p.PID, truncate(p.Name, 15), p.CPUPct,
			formatBytes(p.RSSBytes),
			formatBytes(uint64(p.DiskReadRate)),
			formatBytes(uint64(p.DiskWriteRate)),
			formatBytes(uint64(p.NetRecvRate)),
			formatBytes(uint64(p.NetSendRate)))
	}

	w.Flush()
	fmt.Println(strings.Repeat("─", 100))
}

func (c *CLI) cmdEvents(args []string) {
	n := 20
	if len(args) > 0 {
		if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
			n = num
		}
	}

	events := c.monitor.GetRecentEvents(n)
	if len(events) == 0 {
		fmt.Println("暂无事件记录")
		return
	}

	fmt.Printf("\n最近 %d 条事件:\n", len(events))
	fmt.Println(strings.Repeat("─", 80))

	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		fmt.Printf("[%s] [%s] PID %d (%s): %s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Type, e.PID, e.Name, e.Message)
	}

	fmt.Println(strings.Repeat("─", 80))
}

func (c *CLI) cmdChanges(args []string) {
	n := 20
	if len(args) > 0 {
		if num, err := strconv.Atoi(args[0]); err == nil && num > 0 {
			n = num
		}
	}

	changes := c.monitor.GetProcessChanges(n)
	if len(changes) == 0 {
		fmt.Println("暂无进程变化记录")
		return
	}

	fmt.Printf("\n最近 %d 条进程变化:\n", len(changes))
	fmt.Println(strings.Repeat("─", 80))

	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		typeStr := map[string]string{"new": "新进程", "gone": "消失"}[c.Type]
		fmt.Printf("[%s] [%s] PID %d: %s\n",
			c.Timestamp.Format("2006-01-02 15:04:05"),
			typeStr, c.PID, c.Name)
	}

	fmt.Println(strings.Repeat("─", 80))
}

func (c *CLI) cmdWatch(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: watch <pid>")
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Printf("错误: 无效的 PID '%s'\n", args[0])
		return
	}

	fmt.Printf("实时监控进程 PID %d (按 Ctrl+C 停止)...\n\n", pid)

	// 简单实现：每秒刷新一次
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processes, err := c.monitor.ListAllProcesses()
		if err != nil {
			continue
		}

		var found *types.ProcessInfo
		for i := range processes {
			if processes[i].PID == int32(pid) {
				found = &processes[i]
				break
			}
		}

		// 清屏（简单实现）
		fmt.Print("\033[H\033[2J")

		if found == nil {
			fmt.Printf("进程 PID %d 不存在或已退出\n", pid)
			return
		}

		fmt.Printf("进程: %s (PID %d)\n", found.Name, found.PID)
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("CPU:        %.1f%%\n", found.CPUPct)
		fmt.Printf("内存:       %s (增速: %s/s)\n",
			formatBytes(found.RSSBytes),
			formatMemGrowth(found.RSSGrowthRate))
		fmt.Printf("虚拟内存:   %s\n", formatBytes(found.VMS))
		fmt.Printf("磁盘读:     %s/s\n", formatBytes(uint64(found.DiskReadRate)))
		fmt.Printf("磁盘写:     %s/s\n", formatBytes(uint64(found.DiskWriteRate)))
		fmt.Printf("网络收:     %s/s\n", formatBytes(uint64(found.NetRecvRate)))
		fmt.Printf("网络发:     %s/s\n", formatBytes(uint64(found.NetSendRate)))
		fmt.Printf("运行时长:   %s\n", formatUptime(found.Uptime))
		fmt.Println(strings.Repeat("─", 60))
	}
}

// 辅助函数
func formatBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/1024/1024/1024)
}

func formatMemGrowth(rate float64) string {
	if rate > 0 {
		return fmt.Sprintf("+%s", formatBytes(uint64(rate)))
	} else if rate < 0 {
		return fmt.Sprintf("-%s", formatBytes(uint64(-rate)))
	}
	return "0"
}

func formatUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%d秒", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%d分钟", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%d小时%d分钟", seconds/3600, (seconds%3600)/60)
	}
	return fmt.Sprintf("%d天%d小时", seconds/86400, (seconds%86400)/3600)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func sortByCPU(processes []types.ProcessInfo) {
	for i := 0; i < len(processes)-1; i++ {
		for j := i + 1; j < len(processes); j++ {
			if processes[i].CPUPct < processes[j].CPUPct {
				processes[i], processes[j] = processes[j], processes[i]
			}
		}
	}
}
