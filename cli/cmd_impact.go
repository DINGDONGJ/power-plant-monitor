package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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
	fmt.Println("  set <key> <value>     - 设置影响分析参数")
	fmt.Println("  clear                 - 清除所有影响事件记录")
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("可设置的参数:"))
	fmt.Println("  cpu_threshold         - CPU影响阈值 (0-100)")
	fmt.Println("  memory_threshold      - 内存影响阈值 (0-100)")
	fmt.Println("  io_threshold          - IO影响阈值 (0-100)")
	fmt.Println("  enabled               - 启用/禁用分析 (true/false)")
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
	
	fmt.Println(cmd.cli.formatter.Bold("阈值设置:"))
	fmt.Printf("  CPU阈值:    %s\n", cmd.cli.formatter.FormatPercent(cfg.CPUThreshold))
	fmt.Printf("  内存阈值:   %s\n", cmd.cli.formatter.FormatPercent(cfg.MemoryThreshold))
	fmt.Printf("  IO阈值:     %s\n", cmd.cli.formatter.FormatPercent(cfg.DiskIOThreshold))
	fmt.Println()
	
	fmt.Println(cmd.cli.formatter.Bold("分析参数:"))
	fmt.Printf("  分析周期:   %d秒\n", cfg.AnalysisInterval)
	fmt.Printf("  最大记录:   %d\n", cfg.HistoryLen)
}

func (cmd *ImpactCommand) setConfig(args []string) {
	if len(args) < 2 {
		fmt.Println(cmd.cli.formatter.Error("用法: impact set <key> <value>"))
		fmt.Println(cmd.cli.formatter.Info("可用键: cpu_threshold, memory_threshold, io_threshold, enabled"))
		return
	}

	key := strings.ToLower(args[0])
	value := args[1]

	switch key {
	case "cpu_threshold", "cpu":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cmd.cli.config.Impact.CPUThreshold = v
			fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("CPU阈值已设置为: %.1f%%", v)))
		} else {
			fmt.Println(cmd.cli.formatter.Error("无效的数值"))
		}
	case "memory_threshold", "mem":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cmd.cli.config.Impact.MemoryThreshold = v
			fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("内存阈值已设置为: %.1f%%", v)))
		} else {
			fmt.Println(cmd.cli.formatter.Error("无效的数值"))
		}
	case "io_threshold", "io":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cmd.cli.config.Impact.DiskIOThreshold = v
			fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("IO阈值已设置为: %.1f MB/s", v)))
		} else {
			fmt.Println(cmd.cli.formatter.Error("无效的数值"))
		}
	case "enabled":
		if v, err := strconv.ParseBool(value); err == nil {
			cmd.cli.config.Impact.Enabled = v
			status := "已启用"
			if !v {
				status = "已禁用"
			}
			fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("影响分析%s", status)))
		} else {
			fmt.Println(cmd.cli.formatter.Error("无效的布尔值，请使用 true/false"))
		}
	default:
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("未知配置项: %s", key)))
	}
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
