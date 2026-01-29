package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"monitor-agent/types"
)

// TargetCommand 目标管理命令组
type TargetCommand struct {
	cli *CLI
}

// NewTargetCommand 创建目标命令组
func NewTargetCommand(cli *CLI) *TargetCommand {
	return &TargetCommand{cli: cli}
}

// Handle 处理命令
func (c *TargetCommand) Handle(subCmd string, args []string) {
	switch subCmd {
	case "list", "ls", "":
		c.list(args)
	case "add":
		c.add(args)
	case "remove", "rm":
		c.remove(args)
	case "info":
		c.info(args)
	case "update":
		c.update(args)
	case "clear":
		c.clear()
	default:
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未知子命令: target %s", subCmd)))
		c.PrintHelp()
	}
}

// PrintHelp 打印帮助
func (c *TargetCommand) PrintHelp() {
	fmt.Println(c.cli.formatter.Header("\n目标管理命令 (target):"))
	fmt.Println()
	fmt.Println("  target list [-1]              - 列出监控目标 (默认动态刷新, -1 只显示一次)")
	fmt.Println("  target add <pid|name> [alias] - 添加监控目标")
	fmt.Println("  target remove <pid>           - 移除监控目标")
	fmt.Println("  target info <pid>             - 显示目标详细信息")
	fmt.Println("  target update <pid> <options> - 更新目标配置")
	fmt.Println("  target clear                  - 清除所有监控目标")
	fmt.Println()
	fmt.Println(c.cli.formatter.Bold("update 选项:"))
	fmt.Println("  alias <名称>                  - 设置别名")
	fmt.Println("  add-port <端口>               - 添加监控端口")
	fmt.Println("  add-file <路径>               - 添加监控文件")
	fmt.Println()
	fmt.Println(c.cli.formatter.Info("示例: target add 1234 数据库服务"))
	fmt.Println(c.cli.formatter.Info("示例: target update 1234 add-port 3306"))
}

// list 列出监控目标
func (c *TargetCommand) list(args []string) {
	// 检查是否只显示一次
	onceMode := false
	for _, arg := range args {
		if arg == "-1" || arg == "once" {
			onceMode = true
		}
	}

	if onceMode {
		c.listOnce()
		return
	}

	// 默认动态刷新
	c.listWatch()
}

func (c *TargetCommand) listWatch() {
	fmt.Println(c.cli.formatter.Info("动态监控模式，按 Enter 键退出..."))

	stopChan := make(chan struct{})
	go func() {
		c.cli.scanner.Scan()
		close(stopChan)
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	c.renderTargetList()

	for {
		select {
		case <-stopChan:
			fmt.Println(c.cli.formatter.Info("\n已退出动态监控"))
			return
		case <-ticker.C:
			c.renderTargetList()
		}
	}
}

func (c *TargetCommand) renderTargetList() {
	fmt.Print("\033[H\033[J")
	now := time.Now().Format("15:04:05")

	targets := c.cli.monitor.GetTargets()
	if len(targets) == 0 {
		fmt.Printf("监控目标列表 [%s] 按 Enter 退出\n\n", now)
		fmt.Println(c.cli.formatter.Warning("当前没有监控目标"))
		return
	}

	allProcesses, _ := c.cli.monitor.ListAllProcesses()
	processMap := make(map[int32]*types.ProcessInfo)
	for i := range allProcesses {
		processMap[allProcesses[i].PID] = &allProcesses[i]
	}

	fmt.Printf("监控目标列表 (%d 个) [%s] 按 Enter 退出\n", len(targets), now)
	fmt.Println(strings.Repeat("-", 120))

	table := NewTable("PID", "名称", "别名", "状态", "CPU%", "内存", "内存增速", "磁盘读", "磁盘写", "网络收", "网络发")
	table.PrintHeader()

	for _, t := range targets {
		p, exists := processMap[t.PID]
		status := c.cli.formatter.StatusError("停止")
		cpu, mem, memGrowth := "-", "-", "-"
		diskRead, diskWrite, netRecv, netSend := "-", "-", "-", "-"

		if exists {
			status = c.cli.formatter.StatusOK("运行")
			cpu = FormatPercent(p.CPUPct)
			mem = FormatBytes(p.RSSBytes)
			memGrowth = FormatMemGrowth(p.RSSGrowthRate)
			diskRead = FormatBytesRate(p.DiskReadRate)
			diskWrite = FormatBytesRate(p.DiskWriteRate)
			netRecv = FormatBytesRate(p.NetRecvRate)
			netSend = FormatBytesRate(p.NetSendRate)
		}

		alias := t.Alias
		if alias == "" {
			alias = "-"
		}

		table.AddRow(
			fmt.Sprintf("%d", t.PID),
			Truncate(t.Name, 15),
			Truncate(alias, 10),
			status,
			cpu, mem, memGrowth,
			diskRead, diskWrite, netRecv, netSend,
		)
	}

	table.Flush()
	fmt.Println(strings.Repeat("-", 120))
}

func (c *TargetCommand) listOnce() {
	targets := c.cli.monitor.GetTargets()
	if len(targets) == 0 {
		fmt.Println(c.cli.formatter.Warning("当前没有监控目标"))
		fmt.Println(c.cli.formatter.Info("使用 'target add <pid|name>' 添加目标"))
		return
	}

	// 获取所有进程信息
	allProcesses, err := c.cli.monitor.ListAllProcesses()
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("获取进程信息失败: %v", err)))
		return
	}

	// 构建 PID 映射
	processMap := make(map[int32]*types.ProcessInfo)
	for i := range allProcesses {
		processMap[allProcesses[i].PID] = &allProcesses[i]
	}

	fmt.Println()
	fmt.Println(c.cli.formatter.Header(fmt.Sprintf("监控目标列表 (%d 个)", len(targets))))
	fmt.Println(c.cli.formatter.Divider(120))

	table := NewTable("PID", "名称", "别名", "状态", "CPU%", "内存", "内存增速", "磁盘读", "磁盘写", "网络收", "网络发")
	table.PrintHeader()

	for _, t := range targets {
		p, exists := processMap[t.PID]

		status := c.cli.formatter.StatusError("停止")
		cpu := "-"
		mem := "-"
		memGrowth := "-"
		diskRead := "-"
		diskWrite := "-"
		netRecv := "-"
		netSend := "-"

		if exists {
			status = c.cli.formatter.StatusOK("运行")
			cpu = FormatPercent(p.CPUPct)
			mem = FormatBytes(p.RSSBytes)
			memGrowth = FormatMemGrowth(p.RSSGrowthRate)
			diskRead = FormatBytesRate(p.DiskReadRate)
			diskWrite = FormatBytesRate(p.DiskWriteRate)
			netRecv = FormatBytesRate(p.NetRecvRate)
			netSend = FormatBytesRate(p.NetSendRate)
		}

		alias := t.Alias
		if alias == "" {
			alias = "-"
		}

		table.AddRow(
			fmt.Sprintf("%d", t.PID),
			Truncate(t.Name, 15),
			Truncate(alias, 10),
			status,
			cpu, mem, memGrowth,
			diskRead, diskWrite, netRecv, netSend,
		)
	}

	table.Flush()
	fmt.Println(c.cli.formatter.Divider(120))
}

// add 添加监控目标
func (c *TargetCommand) add(args []string) {
	if len(args) == 0 {
		fmt.Println(c.cli.formatter.Error("用法: target add <pid|name> [alias]"))
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
		processes, err := c.cli.monitor.ListAllProcesses()
		if err != nil {
			fmt.Println(c.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
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
			fmt.Println(c.cli.formatter.Error(fmt.Sprintf("进程 PID %d 不存在", pid)))
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
		processes, err := c.cli.monitor.ListAllProcesses()
		if err != nil {
			fmt.Println(c.cli.formatter.Error(fmt.Sprintf("获取进程列表失败: %v", err)))
			return
		}

		var matches []types.ProcessInfo
		searchName := strings.ToLower(args[0])
		for i := range processes {
			if strings.Contains(strings.ToLower(processes[i].Name), searchName) {
				matches = append(matches, processes[i])
			}
		}

		if len(matches) == 0 {
			fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未找到名称包含 '%s' 的进程", args[0])))
			return
		}

		if len(matches) > 1 {
			fmt.Println(c.cli.formatter.Warning(fmt.Sprintf("找到 %d 个匹配的进程:", len(matches))))
			for _, p := range matches[:min(10, len(matches))] {
				fmt.Printf("  PID %d: %s\n", p.PID, p.Name)
			}
			if len(matches) > 10 {
				fmt.Printf("  ... 还有 %d 个进程\n", len(matches)-10)
			}
			fmt.Println(c.cli.formatter.Info("请使用 PID 精确指定"))
			return
		}

		target = types.MonitorTarget{
			PID:     matches[0].PID,
			Name:    matches[0].Name,
			Alias:   alias,
			Cmdline: matches[0].Cmdline,
		}
	}

	if err := c.cli.monitor.AddTarget(target); err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("添加失败: %v", err)))
		return
	}

	displayName := target.Name
	if target.Alias != "" {
		displayName = fmt.Sprintf("%s (%s)", target.Alias, target.Name)
	}
	fmt.Println(c.cli.formatter.Success(fmt.Sprintf("已添加监控目标: %s [PID %d]", displayName, target.PID)))
}

// remove 移除监控目标
func (c *TargetCommand) remove(args []string) {
	if len(args) == 0 {
		fmt.Println(c.cli.formatter.Error("用法: target remove <pid>"))
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("无效的 PID: %s", args[0])))
		return
	}

	c.cli.monitor.RemoveTarget(int32(pid))
	fmt.Println(c.cli.formatter.Success(fmt.Sprintf("已移除监控目标 PID %d", pid)))
}

// info 显示目标详情
func (c *TargetCommand) info(args []string) {
	if len(args) == 0 {
		fmt.Println(c.cli.formatter.Error("用法: target info <pid>"))
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("无效的 PID: %s", args[0])))
		return
	}

	// 查找目标
	targets := c.cli.monitor.GetTargets()
	var target *types.MonitorTarget
	for i := range targets {
		if targets[i].PID == int32(pid) {
			target = &targets[i]
			break
		}
	}

	if target == nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未找到 PID %d 的监控目标", pid)))
		return
	}

	// 获取进程实时信息
	processes, err := c.cli.monitor.ListAllProcesses()
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("获取进程信息失败: %v", err)))
		return
	}

	var proc *types.ProcessInfo
	for i := range processes {
		if processes[i].PID == int32(pid) {
			proc = &processes[i]
			break
		}
	}

	f := c.cli.formatter
	fmt.Println()
	fmt.Println(f.Header(fmt.Sprintf("监控目标详情 - PID %d", pid)))
	fmt.Println(f.Divider(60))

	// 基本信息
	fmt.Println(f.Bold("\n[基本信息]"))
	fmt.Printf("  PID:            %d\n", target.PID)
	fmt.Printf("  进程名:         %s\n", target.Name)
	if target.Alias != "" {
		fmt.Printf("  别名:           %s\n", target.Alias)
	}
	if target.Cmdline != "" {
		fmt.Printf("  命令行:         %s\n", Truncate(target.Cmdline, 50))
	}

	// 监控配置
	if len(target.WatchPorts) > 0 || len(target.WatchFiles) > 0 {
		fmt.Println(f.Bold("\n[监控配置]"))
		if len(target.WatchPorts) > 0 {
			fmt.Printf("  监控端口:       %v\n", target.WatchPorts)
		}
		if len(target.WatchFiles) > 0 {
			fmt.Printf("  监控文件:       %d 个\n", len(target.WatchFiles))
			for i, file := range target.WatchFiles {
				if i >= 5 {
					fmt.Printf("                  ... 还有 %d 个\n", len(target.WatchFiles)-5)
					break
				}
				fmt.Printf("                  - %s\n", file)
			}
		}
	}

	// 实时状态
	if proc != nil {
		fmt.Println(f.Bold("\n[实时状态]"))
		fmt.Printf("  状态:           %s\n", f.StatusOK("运行中"))
		fmt.Printf("  CPU:            %s\n", FormatPercent(proc.CPUPct))
		fmt.Printf("  内存:           %s\n", FormatBytes(proc.RSSBytes))
		fmt.Printf("  内存增速:       %s\n", FormatMemGrowth(proc.RSSGrowthRate))
		fmt.Printf("  虚拟内存:       %s\n", FormatBytes(proc.VMS))
		fmt.Printf("  线程数:         %d\n", proc.NumThreads)
		fmt.Printf("  句柄数:         %d\n", proc.NumFDs)
		fmt.Printf("  打开文件:       %d\n", proc.OpenFiles)
		fmt.Printf("  磁盘读:         %s\n", FormatBytesRate(proc.DiskReadRate))
		fmt.Printf("  磁盘写:         %s\n", FormatBytesRate(proc.DiskWriteRate))
		fmt.Printf("  网络收:         %s\n", FormatBytesRate(proc.NetRecvRate))
		fmt.Printf("  网络发:         %s\n", FormatBytesRate(proc.NetSendRate))
		fmt.Printf("  运行时长:       %s\n", FormatUptime(proc.Uptime))
	} else {
		fmt.Println(f.Bold("\n[实时状态]"))
		fmt.Printf("  状态:           %s\n", f.StatusError("已停止"))
	}

	fmt.Println(f.Divider(60))
}

// update 更新目标配置
func (c *TargetCommand) update(args []string) {
	if len(args) < 3 {
		fmt.Println(c.cli.formatter.Error("用法: target update <pid> <option> <value>"))
		fmt.Println(c.cli.formatter.Info("选项: alias, add-port, add-file"))
		return
	}

	pid, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("无效的 PID: %s", args[0])))
		return
	}

	// 查找目标
	targets := c.cli.monitor.GetTargets()
	var target *types.MonitorTarget
	for i := range targets {
		if targets[i].PID == int32(pid) {
			target = &targets[i]
			break
		}
	}

	if target == nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未找到 PID %d 的监控目标", pid)))
		return
	}

	option := strings.ToLower(args[1])
	value := args[2]

	switch option {
	case "alias":
		target.Alias = value
	case "add-port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			fmt.Println(c.cli.formatter.Error("无效的端口号"))
			return
		}
		target.WatchPorts = append(target.WatchPorts, port)
	case "add-file":
		target.WatchFiles = append(target.WatchFiles, value)
	default:
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未知选项: %s", option)))
		return
	}

	if err := c.cli.monitor.UpdateTarget(*target); err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("更新失败: %v", err)))
		return
	}

	fmt.Println(c.cli.formatter.Success(fmt.Sprintf("已更新目标 PID %d", pid)))
}

// clear 清除所有监控目标
func (c *TargetCommand) clear() {
	c.cli.monitor.RemoveAllTargets()
	fmt.Println(c.cli.formatter.Success("已清除所有监控目标"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
