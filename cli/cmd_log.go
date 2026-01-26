package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogCommand 日志管理命令组
type LogCommand struct {
	cli *CLI
}

// NewLogCommand 创建日志命令组
func NewLogCommand(c *CLI) *LogCommand {
	return &LogCommand{cli: c}
}

// Handle 处理命令
func (cmd *LogCommand) Handle(subCmd string, args []string) {
	switch subCmd {
	case "tail", "":
		cmd.tailLogs(args)
	case "filter", "f":
		cmd.filterLogs(args)
	case "export", "exp":
		cmd.exportLogs(args)
	case "report", "rpt":
		cmd.generateReport(args)
	case "clear":
		cmd.clearLogs()
	case "files":
		cmd.listLogFiles()
	case "help", "h":
		cmd.PrintHelp()
	default:
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("未知子命令: %s", subCmd)))
		cmd.PrintHelp()
	}
}

// PrintHelp 打印帮助
func (cmd *LogCommand) PrintHelp() {
	fmt.Println(cmd.cli.formatter.Header("\n=== 日志管理命令 (log) ==="))
	fmt.Println()
	fmt.Println("  tail [n]              - 查看最近N条日志 (默认50)")
	fmt.Println("  filter <type>         - 按类型过滤 (METRIC/EVENT/IMPACT)")
	fmt.Println("  export <file>         - 导出日志到文件")
	fmt.Println("  report <file>         - 生成值班运行报告")
	fmt.Println("  files                 - 列出所有日志文件")
	fmt.Println("  clear                 - 清理旧日志文件")
	fmt.Println()
	fmt.Println(cmd.cli.formatter.Info("示例:"))
	fmt.Println("  log tail 100          - 查看最近100条日志")
	fmt.Println("  log filter IMPACT     - 仅显示影响分析日志")
	fmt.Println("  log export report.txt - 导出日志到文件")
	fmt.Println("  log report 日报.txt   - 生成电厂值班运行报告")
}

// LogEntry 日志条目结构
type LogEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       string                 `json:"level"`
	Category    string                 `json:"category"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
	ProcessName string                 `json:"process_name,omitempty"`
	PID         int32                  `json:"pid,omitempty"`
}

func (cmd *LogCommand) tailLogs(args []string) {
	count := 50
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			count = n
		}
	}

	logs := cmd.readRecentLogs(count)
	if len(logs) == 0 {
		fmt.Println(cmd.cli.formatter.Info("暂无日志"))
		return
	}

	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== 最近 %d 条日志 ===", len(logs))))
	fmt.Println()

	for _, log := range logs {
		cmd.printLogEntry(log)
	}

	fmt.Println()
	fmt.Printf(cmd.cli.formatter.Info("显示 %d 条日志\n"), len(logs))
}

func (cmd *LogCommand) filterLogs(args []string) {
	if len(args) == 0 {
		fmt.Println(cmd.cli.formatter.Error("用法: log filter <type>"))
		fmt.Println(cmd.cli.formatter.Info("可用类型: METRIC, EVENT, IMPACT, SERVICE"))
		return
	}

	filterType := strings.ToUpper(args[0])
	count := 50
	if len(args) > 1 {
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			count = n
		}
	}

	allLogs := cmd.readRecentLogs(count * 2) // 读取更多以便过滤
	var filtered []LogEntry

	for _, log := range allLogs {
		if strings.ToUpper(log.Category) == filterType {
			filtered = append(filtered, log)
			if len(filtered) >= count {
				break
			}
		}
	}

	if len(filtered) == 0 {
		fmt.Println(cmd.cli.formatter.Info(fmt.Sprintf("未找到类型为 '%s' 的日志", filterType)))
		return
	}

	fmt.Println(cmd.cli.formatter.Header(fmt.Sprintf("\n=== %s 日志 (共 %d 条) ===", filterType, len(filtered))))
	fmt.Println()

	for _, log := range filtered {
		cmd.printLogEntry(log)
	}
}

func (cmd *LogCommand) exportLogs(args []string) {
	if len(args) == 0 {
		fmt.Println(cmd.cli.formatter.Error("用法: log export <file>"))
		return
	}

	outputFile := args[0]
	count := 1000
	if len(args) > 1 {
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			count = n
		}
	}

	logs := cmd.readRecentLogs(count)
	if len(logs) == 0 {
		fmt.Println(cmd.cli.formatter.Info("暂无日志可导出"))
		return
	}

	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("创建文件失败: %v", err)))
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	writer.WriteString("电厂监控系统日志导出\n")
	writer.WriteString(fmt.Sprintf("导出时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	writer.WriteString(fmt.Sprintf("日志条数: %d\n", len(logs)))
	writer.WriteString(strings.Repeat("=", 80) + "\n\n")

	// 写入日志
	for _, log := range logs {
		line := fmt.Sprintf("[%s] [%s] [%s] %s\n",
			log.Timestamp.Format("2006-01-02 15:04:05"),
			log.Level,
			log.Category,
			log.Message)
		writer.WriteString(line)

		if len(log.Data) > 0 {
			writer.WriteString("  数据: ")
			dataJSON, _ := json.Marshal(log.Data)
			writer.WriteString(string(dataJSON) + "\n")
		}
		writer.WriteString("\n")
	}

	fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("已导出 %d 条日志到: %s", len(logs), outputFile)))
}

func (cmd *LogCommand) listLogFiles() {
	logDir := "logs"
	files, err := os.ReadDir(logDir)
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("读取日志目录失败: %v", err)))
		return
	}

	fmt.Println(cmd.cli.formatter.Header("\n=== 日志文件列表 ==="))
	fmt.Println()

	type fileInfo struct {
		name    string
		size    int64
		modTime time.Time
	}

	var logFiles []fileInfo
	var totalSize int64

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		logFiles = append(logFiles, fileInfo{
			name:    file.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		totalSize += info.Size()
	}

	// 按修改时间排序
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].modTime.After(logFiles[j].modTime)
	})

	fmt.Println(cmd.cli.formatter.Bold(fmt.Sprintf("%-40s %12s %20s", "文件名", "大小", "修改时间")))
	fmt.Println(strings.Repeat("-", 75))

	for _, f := range logFiles {
		fmt.Printf("%-40s %12s %20s\n",
			f.name,
			cmd.cli.formatter.FormatBytes(uint64(f.size)),
			f.modTime.Format("01-02 15:04:05"))
	}

	fmt.Println()
	fmt.Printf(cmd.cli.formatter.Info("共 %d 个文件，总大小: %s\n"),
		len(logFiles),
		cmd.cli.formatter.FormatBytes(uint64(totalSize)))
}

func (cmd *LogCommand) clearLogs() {
	fmt.Print("确认清理7天前的日志文件? (y/n): ")
	if cmd.cli.scanner.Scan() {
		input := strings.ToLower(strings.TrimSpace(cmd.cli.scanner.Text()))
		if input != "y" && input != "yes" {
			fmt.Println(cmd.cli.formatter.Info("操作已取消"))
			return
		}
	}

	logDir := "logs"
	files, err := os.ReadDir(logDir)
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("读取日志目录失败: %v", err)))
		return
	}

	cutoffTime := time.Now().AddDate(0, 0, -7)
	removed := 0
	var freedSize int64

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(logDir, file.Name())
			if err := os.Remove(filePath); err == nil {
				removed++
				freedSize += info.Size()
			}
		}
	}

	if removed > 0 {
		fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("已清理 %d 个日志文件，释放 %s",
			removed,
			cmd.cli.formatter.FormatBytes(uint64(freedSize)))))
	} else {
		fmt.Println(cmd.cli.formatter.Info("没有需要清理的日志文件"))
	}
}

func (cmd *LogCommand) readRecentLogs(count int) []LogEntry {
	logDir := "logs"
	files, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}

	// 找到最新的日志文件
	var latestFile string
	var latestTime time.Time

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if latestFile == "" || info.ModTime().After(latestTime) {
			latestFile = file.Name()
			latestTime = info.ModTime()
		}
	}

	if latestFile == "" {
		return nil
	}

	// 读取日志
	filePath := filepath.Join(logDir, latestFile)
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var logs []LogEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var entry LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			logs = append(logs, entry)
		}
	}

	// 返回最后N条
	if len(logs) > count {
		logs = logs[len(logs)-count:]
	}

	return logs
}

func (cmd *LogCommand) printLogEntry(log LogEntry) {
	timeStr := log.Timestamp.Format("15:04:05")
	levelStr := cmd.formatLevel(log.Level)
	categoryStr := cmd.formatCategory(log.Category)

	fmt.Printf("[%s] %s %s %s\n",
		timeStr,
		levelStr,
		categoryStr,
		log.Message)

	// 如果有进程信息，显示
	if log.ProcessName != "" {
		fmt.Printf("  进程: %s (PID: %d)\n", log.ProcessName, log.PID)
	}

	// 如果有额外数据，显示关键信息
	if len(log.Data) > 0 {
		for k, v := range log.Data {
			// 只显示一些关键字段
			if k == "cpu" || k == "memory" || k == "level" || k == "value" {
				fmt.Printf("  %s: %v\n", k, v)
			}
		}
	}
}

func (cmd *LogCommand) formatLevel(level string) string {
	switch strings.ToUpper(level) {
	case "ERROR":
		return cmd.cli.formatter.Error("[ERROR]")
	case "WARN", "WARNING":
		return cmd.cli.formatter.Warning("[WARN ]")
	case "INFO":
		return cmd.cli.formatter.Info("[INFO ]")
	case "DEBUG":
		return "[DEBUG]"
	default:
		return "[" + level + "]"
	}
}

func (cmd *LogCommand) formatCategory(cat string) string {
	switch strings.ToUpper(cat) {
	case "METRIC":
		return cmd.cli.formatter.Header("[METRIC]")
	case "EVENT":
		return cmd.cli.formatter.Warning("[EVENT ]")
	case "IMPACT":
		return cmd.cli.formatter.Error("[IMPACT]")
	case "SERVICE":
		return cmd.cli.formatter.Info("[SERVICE]")
	default:
		cat = strings.ToUpper(cat)
		if len(cat) < 7 {
			cat = cat + strings.Repeat(" ", 7-len(cat))
		}
		return "[" + cat + "]"
	}
}

// generateReport 生成电厂风格的值班运行报告
func (cmd *LogCommand) generateReport(args []string) {
	if len(args) == 0 {
		fmt.Println(cmd.cli.formatter.Error("用法: log report <file>"))
		fmt.Println(cmd.cli.formatter.Info("示例: log report 日报.txt"))
		return
	}

	outputFile := args[0]
	now := time.Now()

	// 读取所有日志（最近24小时的）
	allLogs := cmd.readRecentLogs(10000)

	// 分类统计
	var eventLogs, impactLogs []LogEntry
	var startCount, exitCount, alertCount int
	severityCount := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}

	for _, log := range allLogs {
		switch strings.ToUpper(log.Category) {
		case "EVENT":
			eventLogs = append(eventLogs, log)
			// 分析事件类型
			msg := strings.ToLower(log.Message)
			if strings.Contains(msg, "start") || strings.Contains(msg, "启动") {
				startCount++
			} else if strings.Contains(msg, "exit") || strings.Contains(msg, "退出") || strings.Contains(msg, "stop") {
				exitCount++
			}
		case "IMPACT":
			impactLogs = append(impactLogs, log)
			// 提取严重级别
			if sev, ok := log.Data["severity"]; ok {
				if sevStr, ok := sev.(string); ok {
					severityCount[strings.ToLower(sevStr)]++
				}
			} else {
				severityCount["medium"]++
			}
		}
	}

	// 获取当前监控目标
	targets := cmd.cli.monitor.GetTargets()

	// 确定值次
	hour := now.Hour()
	var shift string
	if hour >= 8 && hour < 20 {
		shift = "白班 (08:00 - 20:00)"
	} else {
		shift = "夜班 (20:00 - 08:00)"
	}

	// 生成报告
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(cmd.cli.formatter.Error(fmt.Sprintf("创建文件失败: %v", err)))
		return
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	defer w.Flush()

	// 报告头
	w.WriteString("═══════════════════════════════════════════════════════════════\n")
	w.WriteString("              电厂核心软件运行日报\n")
	w.WriteString("═══════════════════════════════════════════════════════════════\n")
	w.WriteString(fmt.Sprintf("单位名称：XX发电厂\n"))
	w.WriteString(fmt.Sprintf("报告日期：%s\n", now.Format("2006-01-02")))
	w.WriteString(fmt.Sprintf("值    次：%s\n", shift))
	w.WriteString(fmt.Sprintf("生成时间：%s\n", now.Format("2006-01-02 15:04:05")))
	w.WriteString("───────────────────────────────────────────────────────────────\n\n")

	// 一、保障软件运行情况
	w.WriteString("一、保障软件运行情况\n")
	if len(targets) == 0 {
		w.WriteString("  暂无保障对象\n")
	} else {
		w.WriteString(fmt.Sprintf("  %-6s %-20s %-8s %-10s %-10s %-10s\n",
			"序号", "软件名称", "状态", "CPU均值", "内存均值", "运行时长"))
		for i, t := range targets {
			// 获取软件状态
			status := "正常"
			cpuAvg := "-"
			memAvg := "-"
			runtime := "-"

			if metrics := cmd.cli.monitor.GetMetrics(t.PID, 100); len(metrics) > 0 {
				// 计算平均值
				var cpuSum, memSum float64
				for _, m := range metrics {
					cpuSum += m.CPUPct
					memSum += float64(m.RSSBytes)
				}
				cpuAvg = fmt.Sprintf("%.1f%%", cpuSum/float64(len(metrics)))
				memAvg = cmd.cli.formatter.FormatBytes(uint64(memSum / float64(len(metrics))))
			}

			displayName := t.Alias
			if displayName == "" {
				displayName = t.Name
			}
			if len(displayName) > 18 {
				displayName = displayName[:18] + ".."
			}

			w.WriteString(fmt.Sprintf("  %-6d %-20s %-8s %-10s %-10s %-10s\n",
				i+1, displayName, status, cpuAvg, memAvg, runtime))
		}
	}
	w.WriteString("\n")

	// 二、运行事件统计
	w.WriteString("二、运行事件统计\n")
	w.WriteString(fmt.Sprintf("  软件启动：%d 次\n", startCount))
	w.WriteString(fmt.Sprintf("  软件退出：%d 次\n", exitCount))
	w.WriteString(fmt.Sprintf("  异常告警：%d 次\n", alertCount))
	w.WriteString("\n")

	// 三、风险事件统计
	w.WriteString("三、风险事件统计\n")
	w.WriteString(fmt.Sprintf("  严重：%-4d 高级：%-4d 中级：%-4d 低级：%d\n",
		severityCount["critical"],
		severityCount["high"],
		severityCount["medium"],
		severityCount["low"]))
	w.WriteString("\n")

	// 四、详细事件记录
	w.WriteString("四、详细事件记录\n")
	if len(impactLogs) == 0 && len(eventLogs) == 0 {
		w.WriteString("  （无）\n")
	} else {
		// 显示最近20条重要事件
		count := 0
		for _, log := range impactLogs {
			if count >= 20 {
				break
			}
			sev := "中级"
			if s, ok := log.Data["severity"]; ok {
				switch strings.ToLower(fmt.Sprintf("%v", s)) {
				case "critical":
					sev = "严重"
				case "high":
					sev = "高级"
				case "medium":
					sev = "中级"
				case "low":
					sev = "低级"
				}
			}
			w.WriteString(fmt.Sprintf("  [%s] [%s] %s\n",
				log.Timestamp.Format("15:04:05"),
				sev,
				log.Message))
			count++
		}
	}
	w.WriteString("\n")

	// 五、值班备注
	w.WriteString("五、值班备注\n")
	w.WriteString("  （无）\n")
	w.WriteString("\n")

	// 报告尾
	w.WriteString("───────────────────────────────────────────────────────────────\n")
	w.WriteString("                    值班员签名：___________\n")
	w.WriteString("═══════════════════════════════════════════════════════════════\n")

	fmt.Println(cmd.cli.formatter.Success(fmt.Sprintf("已生成值班运行报告: %s", outputFile)))
	fmt.Println(cmd.cli.formatter.Info(fmt.Sprintf("  保障软件: %d 个", len(targets))))
	fmt.Println(cmd.cli.formatter.Info(fmt.Sprintf("  运行事件: %d 条", len(eventLogs))))
	fmt.Println(cmd.cli.formatter.Info(fmt.Sprintf("  风险事件: %d 条", len(impactLogs))))
}
