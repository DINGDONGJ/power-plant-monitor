package main

import (
	"flag"
	"fmt"
	"log"

	"monitor-agent/cli"
	"monitor-agent/config"
	"monitor-agent/service"
)

var version = "1.0.0"

func main() {
	var (
		addr        = flag.String("addr", "", "HTTP server address (overrides config)")
		logDir      = flag.String("log-dir", "", "log directory (overrides config)")
		configFile  = flag.String("config", "config.json", "config file path")
		genConfig   = flag.Bool("gen-config", false, "generate example config file")
		showVersion = flag.Bool("version", false, "show version")
	)
	flag.Parse()

	// 显示版本
	if *showVersion {
		fmt.Printf("Monitor Agent v%s\n", version)
		return
	}

	// 生成示例配置
	if *genConfig {
		if err := config.GenerateExampleConfig(*configFile); err != nil {
			log.Fatalf("Generate config failed: %v", err)
		}
		fmt.Printf("Example config generated: %s\n", *configFile)
		return
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Load config failed: %v", err)
	}

	// 命令行参数覆盖配置
	if *addr != "" {
		cfg.Server.Addr = *addr
	}
	if *logDir != "" {
		cfg.Logging.Dir = *logDir
	}

	// 转换为服务配置
	serviceCfg := service.Config{
		Addr:       cfg.Server.Addr,
		LogDir:     cfg.Logging.Dir,
		ConfigFile: *configFile,
	}

	// 启动 CLI + Web 模式
	runCLIWithWeb(serviceCfg, cfg)
}

func runCLIWithWeb(serviceCfg service.Config, cfg *config.Config) {
	s, err := service.NewWithConfig(serviceCfg, cfg)
	if err != nil {
		log.Fatalf("Create service failed: %v", err)
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Start failed: %v", err)
	}

	// 显示启动信息
	fmt.Println("Monitor Agent started")
	fmt.Printf("Web interface: http://localhost%s\n", cfg.Server.Addr)
	fmt.Printf("Monitoring %d targets\n", len(cfg.Targets))
	fmt.Println("提示: 输入 'log console on' 可开启终端日志输出")
	fmt.Println()

	// 启动 CLI（在前台运行）
	cliInterface := cli.NewCLI(s.GetMonitor(), serviceCfg.ConfigFile, cfg)
	cliInterface.Run()

	// CLI 退出后停止服务
	s.Stop()
}
