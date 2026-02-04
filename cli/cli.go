package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"monitor-agent/config"
	"monitor-agent/monitor"
)

// CLI 命令行交互界面
type CLI struct {
	monitor    *monitor.MultiMonitor
	configFile string
	config     *config.Config
	scanner    *bufio.Scanner
	formatter  *Formatter
	running    bool

	// 命令组
	configCmd *ConfigCommand
	targetCmd *TargetCommand
	impactCmd *ImpactCommand
	systemCmd *SystemCommand
	logCmd    *LogCommand
}

// NewCLI 创建命令行界面
func NewCLI(m *monitor.MultiMonitor, configFile string, cfg *config.Config) *CLI {
	cli := &CLI{
		monitor:    m,
		configFile: configFile,
		config:     cfg,
		scanner:    bufio.NewScanner(os.Stdin),
		formatter:  NewFormatter(),
		running:    true,
	}

	// 初始化命令组
	cli.configCmd = NewConfigCommand(cli)
	cli.targetCmd = NewTargetCommand(cli)
	cli.impactCmd = NewImpactCommand(cli)
	cli.systemCmd = NewSystemCommand(cli)
	cli.logCmd = NewLogCommand(cli)

	return cli
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
	fmt.Println(c.formatter.Header("╔═══════════════════════════════════════════════════════════╗"))
	fmt.Println(c.formatter.Header("║    电厂核心软件监视保障系统 - 命令行管理界面              ║"))
	fmt.Println(c.formatter.Header("║    Monitor Agent CLI v2.0                                 ║"))
	fmt.Println(c.formatter.Header("╚═══════════════════════════════════════════════════════════╝"))
}

// ShowMainScreen 显示主界面（清屏后显示banner和帮助）
func (c *CLI) ShowMainScreen() {
	fmt.Print("\033[H\033[2J") // 清屏
	c.printBanner()
	c.printHelp()
}

func (c *CLI) printHelp() {
	fmt.Println("\n" + c.formatter.Bold("命令分组:"))
	fmt.Println()

	fmt.Println(c.formatter.Header("  配置管理 (config):"))
	fmt.Println("    config show                     - 显示当前配置")
	fmt.Println("    config set <key> <value>        - 设置配置项 (自动保存)")
	fmt.Println("    config save                     - 手动保存配置到文件")
	fmt.Println("    config reload                   - 重新加载配置")
	fmt.Println()

	fmt.Println(c.formatter.Header("  目标管理 (target):"))
	fmt.Println("    target list                     - 列出所有监控目标 (动态刷新)")
	fmt.Println("    target list -1                  - 列出所有监控目标 (只显示一次)")
	fmt.Println("    target add <pid|name> [alias]   - 添加监控目标 (自动保存)")
	fmt.Println("    target remove <pid>             - 移除监控目标 (自动保存)")
	fmt.Println("    target info <pid>               - 显示目标详情")
	fmt.Println("    target update <pid> <key> <val> - 更新目标配置 (自动保存)")
	fmt.Println("    target clear                    - 清除所有目标 (自动保存)")
	fmt.Println()

	fmt.Println(c.formatter.Header("  影响分析 (impact):"))
	fmt.Println("    impact list [n]                 - 显示影响事件 (默认20)")
	fmt.Println("    impact summary                  - 显示影响统计")
	fmt.Println("    impact config                   - 显示影响分析配置")
	fmt.Println("    impact set <key> <value>        - 设置影响分析参数 (自动保存)")
	fmt.Println("    impact clear                    - 清除所有影响事件")
	fmt.Println()

	fmt.Println(c.formatter.Header("  系统信息 (system):"))
	fmt.Println("    system status                   - 显示系统状态 (动态刷新)")
	fmt.Println("    system status -1                - 显示系统状态 (只显示一次)")
	fmt.Println("    system top [n]                  - 显示Top进程 (默认10)")
	fmt.Println("    system ps [pattern]             - 列出进程")
	fmt.Println("    system events [n]               - 显示事件 (默认20)")
	fmt.Println("    system watch <pid>              - 实时监控进程")
	fmt.Println()

	fmt.Println(c.formatter.Header("  日志管理 (log):"))
	fmt.Println("    log console [on|off]            - 启停终端日志输出")
	fmt.Println("    log tail [n]                    - 查看最近N条日志 (默认50)")
	fmt.Println("    log filter <type>               - 按类型过滤 (METRIC/EVENT/IMPACT)")
	fmt.Println("    log export <file>               - 导出日志")
	fmt.Println()

	fmt.Println(c.formatter.Header("  通用命令:"))
	fmt.Println("    help, ?                         - 显示帮助")
	fmt.Println("    clear, cls                      - 清屏")
	fmt.Println("    exit, quit                      - 退出")
	fmt.Println()
	fmt.Println(c.formatter.Info("提示: 配置修改会自动保存到 config.json，CLI 和 Web 数据实时同步"))
}

func (c *CLI) handleCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	cmdGroup := strings.ToLower(parts[0])
	subCmd := ""
	args := []string{}

	if len(parts) > 1 {
		subCmd = strings.ToLower(parts[1])
		if len(parts) > 2 {
			args = parts[2:]
		}
	}

	switch cmdGroup {
	case "config", "cfg":
		c.configCmd.Handle(subCmd, args)
	case "target", "tgt":
		c.targetCmd.Handle(subCmd, args)
	case "impact", "imp":
		c.impactCmd.Handle(subCmd, args)
	case "system", "sys":
		c.systemCmd.Handle(subCmd, args)
	case "log":
		c.logCmd.Handle(subCmd, args)

	// 通用命令
	case "help", "h", "?":
		if subCmd != "" {
			c.printCommandHelp(subCmd)
		} else {
			c.printHelp()
		}
	case "clear", "cls":
		fmt.Print("\033[H\033[2J")
	case "exit", "quit", "q":
		c.running = false
		fmt.Println(c.formatter.Info("再见!"))

	default:
		fmt.Println(c.formatter.Error(fmt.Sprintf("未知命令: %s", cmdGroup)))
		fmt.Println(c.formatter.Info("输入 'help' 查看可用命令"))
	}
}

func (c *CLI) printCommandHelp(cmdGroup string) {
	switch cmdGroup {
	case "config", "cfg":
		c.configCmd.PrintHelp()
	case "target", "tgt":
		c.targetCmd.PrintHelp()
	case "impact", "imp":
		c.impactCmd.PrintHelp()
	case "system", "sys":
		c.systemCmd.PrintHelp()
	case "log":
		c.logCmd.PrintHelp()
	default:
		fmt.Println(c.formatter.Error(fmt.Sprintf("未知命令组: %s", cmdGroup)))
		c.printHelp()
	}
}
