package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"monitor-agent/cli"
	"monitor-agent/config"
	"monitor-agent/service"
)

var version = "1.0.0"

func main() {
	var (
		addr       = flag.String("addr", "", "HTTP server address (overrides config)")
		logDir     = flag.String("log-dir", "", "log directory (overrides config)")
		configFile = flag.String("config", "config.json", "config file path")
		cliMode    = flag.Bool("cli", false, "run CLI interactive mode (Web server depends on config)")
		cliOnly    = flag.Bool("cli-only", false, "run CLI only mode (disable Web server)")
		genConfig  = flag.Bool("gen-config", false, "generate example config file")

		// 服务管理命令
		runService  = flag.Bool("service", false, "run as service")
		install     = flag.Bool("install", false, "install as system service")
		uninstall   = flag.Bool("uninstall", false, "uninstall system service")
		start       = flag.Bool("start", false, "start the service")
		stop        = flag.Bool("stop", false, "stop the service")
		status      = flag.Bool("status", false, "show service status")
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

	// 服务管理命令
	if *install {
		if err := service.InstallService(); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("Service installed successfully")
		return
	}

	if *uninstall {
		if err := service.UninstallService(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("Service uninstalled successfully")
		return
	}

	if *start {
		if err := service.StartService(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		return
	}

	if *stop {
		if err := service.StopService(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		return
	}

	if *status {
		s, err := service.ServiceStatus()
		if err != nil {
			log.Fatalf("Status check failed: %v", err)
		}
		fmt.Printf("Service status: %s\n", s)
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
	
	// -cli-only 强制禁用 Web 服务器
	if *cliOnly {
		cfg.Server.Enabled = false
		*cliMode = true
	}

	// 转换为服务配置
	serviceCfg := service.Config{
		Addr:       cfg.Server.Addr,
		LogDir:     cfg.Logging.Dir,
		ConfigFile: *configFile,
	}

	// CLI 模式（可以和 Web 同时运行）
	if *cliMode {
		runCLIWithOptionalWeb(serviceCfg, cfg)
		return
	}

	// 运行服务
	if *runService {
		// 以服务模式运行
		if err := service.RunAsService(serviceCfg); err != nil {
			log.Fatalf("Service error: %v", err)
		}
	} else {
		// 交互式运行
		runInteractive(serviceCfg, cfg)
	}
}

func runInteractive(serviceCfg service.Config, cfg *config.Config) {
	s, err := service.NewWithConfig(serviceCfg, cfg)
	if err != nil {
		log.Fatalf("Create service failed: %v", err)
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Start failed: %v", err)
	}

	fmt.Println("Monitor Agent running in interactive mode")
	fmt.Println("Press Ctrl+C to stop")
	if cfg.Server.Enabled {
		fmt.Printf("Web interface: http://localhost%s\n", cfg.Server.Addr)
	} else {
		fmt.Println("Web interface: disabled")
	}
	fmt.Printf("Monitoring %d targets\n", len(cfg.Targets))

	// 等待信号
	waitForSignal()

	s.Stop()
}

func runCLIWithOptionalWeb(serviceCfg service.Config, cfg *config.Config) {
	s, err := service.NewWithConfig(serviceCfg, cfg)
	if err != nil {
		log.Fatalf("Create service failed: %v", err)
	}

	if err := s.Start(); err != nil {
		log.Fatalf("Start failed: %v", err)
	}

	// 显示启动信息
	fmt.Println("Monitor Agent started")
	if cfg.Server.Enabled {
		fmt.Printf("Web interface: http://localhost%s (running in background)\n", cfg.Server.Addr)
		fmt.Println("You can access the Web UI while using CLI")
	} else {
		fmt.Println("Web interface: disabled")
	}
	fmt.Printf("Monitoring %d targets\n", len(cfg.Targets))
	fmt.Println()

	// 启动 CLI（在前台运行）
	cliInterface := cli.NewCLI(s.GetMonitor(), serviceCfg.ConfigFile, cfg)
	cliInterface.Run()

	// CLI 退出后停止服务
	s.Stop()
}

func waitForSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
}
