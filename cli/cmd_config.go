package cli

import (
	"fmt"
	"strconv"
	"strings"

	"monitor-agent/config"
)

// ConfigCommand 配置管理命令组
type ConfigCommand struct {
	cli *CLI
}

// NewConfigCommand 创建配置命令组
func NewConfigCommand(cli *CLI) *ConfigCommand {
	return &ConfigCommand{cli: cli}
}

// Handle 处理命令
func (c *ConfigCommand) Handle(subCmd string, args []string) {
	switch subCmd {
	case "show", "":
		c.show()
	case "set":
		c.set(args)
	case "save":
		c.save()
	case "reload":
		c.reload()
	default:
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("未知子命令: config %s", subCmd)))
		c.PrintHelp()
	}
}

// PrintHelp 打印帮助
func (c *ConfigCommand) PrintHelp() {
	fmt.Println(c.cli.formatter.Header("\n配置管理命令 (config):"))
	fmt.Println()
	fmt.Println("  config show                   - 显示当前配置")
	fmt.Println("  config set <key> <value>      - 设置配置项")
	fmt.Println("  config save                   - 保存配置到文件")
	fmt.Println("  config reload                 - 重新加载配置")
	fmt.Println()
	fmt.Println(c.cli.formatter.Bold("可设置的配置项:"))
	fmt.Println("  基础配置:")
	fmt.Println("    interval <秒>               - 采样间隔")
	fmt.Println("    server.addr <地址>          - Web服务地址 (如 :8080)")
	fmt.Println("    server.enabled <true|false> - Web服务开关")
	fmt.Println()
	fmt.Println("  系统级阈值:")
	fmt.Println("    cpu-threshold <百分比>      - 系统CPU阈值")
	fmt.Println("    memory-threshold <百分比>   - 系统内存阈值")
	fmt.Println("    disk-threshold <MB/s>       - 系统磁盘IO阈值")
	fmt.Println("    network-threshold <MB/s>    - 系统网络阈值")
	fmt.Println()
	fmt.Println("  进程级阈值:")
	fmt.Println("    proc-cpu <百分比>           - 进程CPU阈值")
	fmt.Println("    proc-mem <MB>               - 进程内存阈值")
	fmt.Println("    proc-threads <数量>         - 进程线程数阈值")
	fmt.Println("    proc-fds <数量>             - 进程句柄数阈值")
	fmt.Println("    proc-disk-read <MB/s>       - 进程磁盘读阈值")
	fmt.Println("    proc-disk-write <MB/s>      - 进程磁盘写阈值")
	fmt.Println("    proc-net-recv <MB/s>        - 进程网络收阈值")
	fmt.Println("    proc-net-send <MB/s>        - 进程网络发阈值")
	fmt.Println()
	fmt.Println(c.cli.formatter.Info("示例: config set interval 3"))
	fmt.Println(c.cli.formatter.Info("示例: config set proc-cpu 60"))
}

// show 显示当前配置
func (c *ConfigCommand) show() {
	cfg := c.cli.config
	f := c.cli.formatter

	fmt.Println()
	fmt.Println(f.Header("当前配置"))
	fmt.Println(f.Divider(60))
	
	// 基础配置
	fmt.Println(f.Bold("\n[基础配置]"))
	fmt.Printf("  配置文件:       %s\n", c.cli.configFile)
	fmt.Printf("  采样间隔:       %d 秒\n", cfg.Sampling.Interval)
	fmt.Printf("  Web服务:        %s (地址: %s)\n", 
		map[bool]string{true: f.StatusOK("启用"), false: f.StatusError("禁用")}[cfg.Server.Enabled],
		cfg.Server.Addr)
	fmt.Printf("  日志目录:       %s\n", cfg.Logging.Dir)
	fmt.Printf("  控制台日志:     %s\n", map[bool]string{true: "是", false: "否"}[cfg.Logging.ConsoleOutput])
	fmt.Printf("  文件日志:       %s\n", map[bool]string{true: "是", false: "否"}[cfg.Logging.FileOutput])
	
	// 影响分析配置
	fmt.Println(f.Bold("\n[影响分析]"))
	fmt.Printf("  功能状态:       %s\n", 
		map[bool]string{true: f.StatusOK("启用"), false: f.StatusError("禁用")}[cfg.Impact.Enabled])
	fmt.Printf("  分析间隔:       %d 秒\n", cfg.Impact.AnalysisInterval)
	fmt.Printf("  Top进程数:      %d\n", cfg.Impact.TopNProcesses)
	
	// 系统级阈值
	fmt.Println(f.Bold("\n[系统级阈值]"))
	fmt.Printf("  CPU:            %.0f%%\n", cfg.Impact.CPUThreshold)
	fmt.Printf("  内存:           %.0f%%\n", cfg.Impact.MemoryThreshold)
	fmt.Printf("  磁盘IO:         %.0f MB/s\n", cfg.Impact.DiskIOThreshold)
	fmt.Printf("  网络:           %.0f MB/s\n", cfg.Impact.NetworkThreshold)
	
	// 进程级阈值
	fmt.Println(f.Bold("\n[进程级阈值] (0=禁用检测)"))
	fmt.Printf("  CPU:            %.0f%%\n", cfg.Impact.ProcCPUThreshold)
	fmt.Printf("  内存:           %.0f MB\n", cfg.Impact.ProcMemoryThreshold)
	fmt.Printf("  内存增速:       %.0f MB/s\n", cfg.Impact.ProcMemGrowthThreshold)
	fmt.Printf("  虚拟内存:       %.0f MB\n", cfg.Impact.ProcVMSThreshold)
	fmt.Printf("  线程数:         %d\n", cfg.Impact.ProcThreadsThreshold)
	fmt.Printf("  句柄数:         %d\n", cfg.Impact.ProcFDsThreshold)
	fmt.Printf("  打开文件数:     %d\n", cfg.Impact.ProcOpenFilesThreshold)
	fmt.Printf("  磁盘读:         %.0f MB/s\n", cfg.Impact.ProcDiskReadThreshold)
	fmt.Printf("  磁盘写:         %.0f MB/s\n", cfg.Impact.ProcDiskWriteThreshold)
	fmt.Printf("  网络收:         %.0f MB/s\n", cfg.Impact.ProcNetRecvThreshold)
	fmt.Printf("  网络发:         %.0f MB/s\n", cfg.Impact.ProcNetSendThreshold)
	
	// 资源检测间隔
	fmt.Println(f.Bold("\n[资源检测间隔]"))
	fmt.Printf("  文件检测:       %d 秒\n", cfg.Impact.FileCheckInterval)
	fmt.Printf("  端口检测:       %d 秒\n", cfg.Impact.PortCheckInterval)
	
	fmt.Println(f.Divider(60))
	fmt.Println(f.Info("使用 'config set <key> <value>' 修改配置"))
}

// set 设置配置项
func (c *ConfigCommand) set(args []string) {
	if len(args) < 2 {
		fmt.Println(c.cli.formatter.Error("用法: config set <key> <value>"))
		return
	}

	key := strings.ToLower(args[0])
	value := args[1]
	cfg := c.cli.config
	f := c.cli.formatter

	var err error
	var changed bool

	switch key {
	// 基础配置
	case "interval":
		var v int
		if v, err = strconv.Atoi(value); err == nil && v > 0 {
			cfg.Sampling.Interval = v
			changed = true
		} else {
			err = fmt.Errorf("间隔必须是正整数")
		}
	case "server.addr":
		cfg.Server.Addr = value
		changed = true
	case "server.enabled":
		cfg.Server.Enabled = value == "true" || value == "1"
		changed = true

	// 系统级阈值
	case "cpu-threshold":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v > 0 {
			cfg.Impact.CPUThreshold = v
			changed = true
		}
	case "memory-threshold":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v > 0 {
			cfg.Impact.MemoryThreshold = v
			changed = true
		}
	case "disk-threshold":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.DiskIOThreshold = v
			changed = true
		}
	case "network-threshold":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.NetworkThreshold = v
			changed = true
		}

	// 进程级阈值
	case "proc-cpu":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcCPUThreshold = v
			changed = true
		}
	case "proc-mem":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcMemoryThreshold = v
			changed = true
		}
	case "proc-threads":
		var v int
		if v, err = strconv.Atoi(value); err == nil && v >= 0 {
			cfg.Impact.ProcThreadsThreshold = v
			changed = true
		}
	case "proc-fds":
		var v int
		if v, err = strconv.Atoi(value); err == nil && v >= 0 {
			cfg.Impact.ProcFDsThreshold = v
			changed = true
		}
	case "proc-disk-read":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcDiskReadThreshold = v
			changed = true
		}
	case "proc-disk-write":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcDiskWriteThreshold = v
			changed = true
		}
	case "proc-net-recv":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcNetRecvThreshold = v
			changed = true
		}
	case "proc-net-send":
		var v float64
		if v, err = strconv.ParseFloat(value, 64); err == nil && v >= 0 {
			cfg.Impact.ProcNetSendThreshold = v
			changed = true
		}

	default:
		fmt.Println(f.Error(fmt.Sprintf("未知配置项: %s", key)))
		fmt.Println(f.Info("使用 'help config' 查看可用配置项"))
		return
	}

	if err != nil {
		fmt.Println(f.Error(fmt.Sprintf("无效的值: %v", err)))
		return
	}

	if changed {
		// 更新影响分析器配置
		if analyzer := c.cli.monitor.GetImpactAnalyzer(); analyzer != nil {
			analyzer.UpdateConfig(cfg.Impact)
		}
		fmt.Println(f.Success(fmt.Sprintf("已设置 %s = %s", key, value)))
		fmt.Println(f.Info("使用 'config save' 保存到文件"))
	}
}

// save 保存配置
func (c *ConfigCommand) save() {
	if err := config.SaveConfig(c.cli.configFile, c.cli.config); err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("保存失败: %v", err)))
		return
	}
	fmt.Println(c.cli.formatter.Success(fmt.Sprintf("配置已保存到 %s", c.cli.configFile)))
}

// reload 重新加载配置
func (c *ConfigCommand) reload() {
	cfg, err := config.LoadConfig(c.cli.configFile)
	if err != nil {
		fmt.Println(c.cli.formatter.Error(fmt.Sprintf("加载失败: %v", err)))
		return
	}
	
	c.cli.config = cfg
	
	// 更新影响分析器配置
	if analyzer := c.cli.monitor.GetImpactAnalyzer(); analyzer != nil {
		analyzer.UpdateConfig(cfg.Impact)
	}
	
	fmt.Println(c.cli.formatter.Success("配置已重新加载"))
}
