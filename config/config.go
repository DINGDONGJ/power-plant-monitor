package config

import (
	"encoding/json"
	"fmt"
	"os"

	"monitor-agent/types"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig          `json:"server"`
	Logging  LoggingConfig         `json:"logging"`
	Targets  []types.MonitorTarget `json:"targets"`
	Sampling SamplingConfig        `json:"sampling"`
	Impact   types.ImpactConfig    `json:"impact"` // 影响分析配置
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Addr    string `json:"addr"`
	Enabled bool   `json:"enabled"` // 是否启用 Web 服务
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Dir             string `json:"dir"`
	Level           string `json:"level"` // debug, info, warn, error
	ConsoleOutput   bool   `json:"console_output"`
	FileOutput      bool   `json:"file_output"`
	EventsToConsole bool   `json:"events_to_console"` // 是否将事件输出到控制台
}

// SamplingConfig 采样配置
type SamplingConfig struct {
	Interval         int `json:"interval"`          // 采样间隔（秒）
	MetricsBufferLen int `json:"metrics_buffer_len"` // 指标缓冲区大小
	EventsBufferLen  int `json:"events_buffer_len"`  // 事件缓冲区大小
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:    ":8080",
			Enabled: true,
		},
		Logging: LoggingConfig{
			Dir:             "./logs",
			Level:           "info",
			ConsoleOutput:   true,
			FileOutput:      true,
			EventsToConsole: true,
		},
		Targets: []types.MonitorTarget{},
		Sampling: SamplingConfig{
			Interval:         1,
			MetricsBufferLen: 300,
			EventsBufferLen:  100,
		},
		Impact: types.ImpactConfig{
			Enabled:          true,
			AnalysisInterval: 5,
			TopNProcesses:    10,
			HistoryLen:       100,
			// 系统级别阈值
			CPUThreshold:     80,
			MemoryThreshold:  85,
			DiskIOThreshold:  100,
			NetworkThreshold: 100,
			// 进程级别阈值
			ProcCPUThreshold:       50,
			ProcMemoryThreshold:    1000,
			ProcMemGrowthThreshold: 10,
			ProcVMSThreshold:       0,
			ProcFDsThreshold:       1000,
			ProcThreadsThreshold:   500,
			ProcOpenFilesThreshold: 500,
			ProcDiskReadThreshold:  50,
			ProcDiskWriteThreshold: 50,
			ProcNetRecvThreshold:   50,
			ProcNetSendThreshold:   50,
			// 资源冲突检测间隔
			FileCheckInterval: 30,
			PortCheckInterval: 30,
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	// 如果文件不存在，返回默认配置
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return cfg, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// GenerateExampleConfig 生成示例配置文件
func GenerateExampleConfig(path string) error {
	cfg := DefaultConfig()
	cfg.Targets = []types.MonitorTarget{
		{
			PID:        0,
			Name:       "nginx",
			Alias:      "Web服务器",
			WatchPorts: []int{80, 443},
		},
		{
			PID:        0,
			Name:       "mysql",
			Alias:      "数据库服务",
			WatchPorts: []int{3306},
			WatchFiles: []string{"/etc/mysql/my.cnf"},
		},
	}

	return SaveConfig(path, cfg)
}
