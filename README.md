# 电厂核心软件监视保障系统

一个轻量级的进程监控代理系统，提供实时进程监控、影响分析和管理界面。支持 Windows 和 Linux 双平台，可部署为系统服务实现 7×24 小时无人值守运行。

## 项目背景

在电厂等关键基础设施环境中，核心软件的稳定运行至关重要。本系统旨在：
- 实时监控关键进程的运行状态
- 采集系统和进程级别的详细性能指标
- 分析其他进程对监控目标的潜在影响
- 检测进程启动/退出等状态变化
- 提供 Web 界面和 CLI 两种管理方式

## 快速开始

### 环境要求
- Go 1.19 或更高版本
- Windows 7+ 或 Linux（支持 systemd）
- 管理员/root 权限（用于网络包捕获）

### 编译依赖

进程网络流量监控使用 gopacket 抓包，需要安装 pcap 库：

**Linux**：
```bash
sudo apt install libpcap-dev
```

**Windows**：
- 下载安装 [Npcap](https://npcap.com/)
- 安装时勾选 "Install Npcap in WinPcap API-compatible Mode"

### 编译

```bash
# 克隆项目
git clone <repository-url>
cd monitor-agent

# 编译 Windows 版本
go build -o bin/monitor-agent.exe ./cmd/web

# 编译 Linux 版本
GOOS=linux go build -o bin/monitor-agent ./cmd/web
```

### 配置

生成配置文件：
```bash
./monitor-agent.exe -gen-config
```

编辑 `config.json`：
```json
{
  "server": {
    "addr": ":8080",
    "enabled": true
  },
  "targets": [
    {
      "pid": 0,
      "name": "your-process.exe",
      "alias": "核心进程"
    }
  ],
  "sampling": {
    "interval": 1,
    "metrics_buffer_len": 300,
    "events_buffer_len": 100
  },
  "impact": {
    "enabled": true,
    "cpu_threshold": 80,
    "memory_threshold": 85,
    "analysis_interval": 5
  }
}
```

---

## 运行模式

### 模式一：Web UI 模式（推荐生产环境）

```bash
./monitor-agent.exe -config config.json
```

- 启动 Web 服务器，访问 `http://localhost:8080` 查看监控界面
- 适合远程监控和可视化展示
- 按 `Ctrl+C` 退出

### 模式二：CLI + Web 同时运行

```bash
./monitor-agent.exe -cli -config config.json
```

- CLI 在前台运行，可交互操作
- Web 服务器在后台运行
- **两者共享同一个监控实例，数据完全同步**
- 在 CLI 中的操作，Web 界面立即可见

### 模式三：纯 CLI 模式

```bash
./monitor-agent.exe -cli-only -config config.json
```

- 禁用 Web 服务器，仅运行 CLI
- 适合服务器环境或不需要远程访问的场景

---

## CLI 命令参考

CLI 采用命令组架构，每个命令组包含多个子命令。

### 配置管理 (config)

| 命令 | 说明 |
|------|------|
| `config show` | 显示当前配置（包括采样、阈值等） |
| `config set <key> <value>` | 设置配置项 |
| `config save` | 保存配置到文件 |
| `config reload` | 重新加载配置 |

**可设置的配置项**：
- `interval` - 采样间隔（秒）
- `cpu-threshold` - 系统 CPU 阈值（%）
- `memory-threshold` - 系统内存阈值（%）
- `proc-cpu` - 进程 CPU 阈值（%）
- `proc-mem` - 进程内存阈值（MB）

### 目标管理 (target)

| 命令 | 说明 | 示例 |
|------|------|------|
| `target list` | 列出所有监控目标 | `target list` |
| `target add <pid\|name> [alias]` | 添加监控目标 | `target add nginx Web服务` |
| `target remove <pid>` | 移除监控目标 | `target remove 1234` |
| `target info <pid>` | 显示目标详情 | `target info 1234` |
| `target update <pid> <key> <val>` | 更新目标配置 | `target update 1234 alias 新名称` |
| `target clear` | 清除所有目标 | `target clear` |

### 影响分析 (impact)

| 命令 | 说明 |
|------|------|
| `impact list [n]` | 显示影响事件（默认20条） |
| `impact summary` | 显示影响统计汇总 |
| `impact config` | 显示影响分析配置 |
| `impact set <key> <value>` | 设置影响分析参数 |
| `impact clear` | 清除所有影响事件 |

### 系统信息 (system)

| 命令 | 说明 | 示例 |
|------|------|------|
| `system status` | 显示系统整体状态 | `system status` |
| `system top [n]` | 显示 Top N 进程（按 CPU） | `system top 20` |
| `system ps [pattern]` | 列出进程（可过滤） | `system ps java` |
| `system events [n]` | 显示最近事件 | `system events 50` |
| `system watch <pid>` | 实时监控进程（60秒） | `system watch 1234` |

### 日志管理 (log)

| 命令 | 说明 |
|------|------|
| `log tail [n]` | 查看最近 N 条日志（默认50） |
| `log filter <type>` | 按类型过滤（METRIC/EVENT/IMPACT） |
| `log export <file>` | 导出日志到文件 |
| `log files` | 列出所有日志文件 |
| `log clear` | 清理 7 天前的日志 |

### 通用命令

| 命令 | 说明 |
|------|------|
| `help` 或 `?` | 显示帮助 |
| `help <command>` | 显示指定命令组帮助 |
| `clear` 或 `cls` | 清屏 |
| `exit` 或 `quit` | 退出程序 |

### CLI 快捷别名

- `config` → `cfg`
- `target` → `tgt`
- `impact` → `imp`
- `system` → `sys`

---

## Web UI 功能

### 主界面
- 系统资源实时曲线图（CPU/内存/网络），60秒历史数据
- 监控目标列表，显示关键指标和状态
- 影响事件实时显示

### 进程列表
- 实时显示系统所有进程
- 支持按名称、PID、用户搜索
- 同名进程自动分组
- 可自定义显示列，拖拽调整顺序

### 监控管理
- 点击进程即可添加到监控
- 支持设置别名、监控端口、监控文件
- 实时显示监控目标的性能指标

### 影响分析
- 自动检测对监控目标的潜在影响
- 显示影响类型、严重程度、来源进程
- 提供处理建议

---

## 命令行参数

| 参数 | 说明 |
|------|------|
| `-config <file>` | 指定配置文件（默认：config.json） |
| `-cli` | CLI 模式（Web 根据配置决定是否启动） |
| `-cli-only` | 纯 CLI 模式（禁用 Web） |
| `-gen-config` | 生成示例配置文件 |
| `-addr <addr>` | 覆盖服务器地址（如 `:8080`） |
| `-log-dir <dir>` | 覆盖日志目录 |
| `-service` | 以服务模式运行 |
| `-install` | 安装系统服务 |
| `-uninstall` | 卸载系统服务 |
| `-start` | 启动服务 |
| `-stop` | 停止服务 |
| `-status` | 查看服务状态 |
| `-version` | 显示版本信息 |

---

## 影响分析功能

### 检测类型

| 类型 | 说明 |
|------|------|
| CPU 竞争 | 其他进程占用大量 CPU，影响监控目标 |
| 内存压力 | 系统内存不足或其他进程内存占用过高 |
| 磁盘 IO | 其他进程磁盘读写影响监控目标 |
| 网络 IO | 其他进程网络流量影响监控目标 |
| 端口冲突 | 其他进程占用监控目标的端口 |
| 文件冲突 | 其他进程访问监控目标的关键文件 |

### 严重级别

- **critical** - 严重影响，需立即处理
- **high** - 高影响，建议尽快处理
- **medium** - 中等影响，建议关注
- **low** - 轻微影响

### 阈值配置

在 `config.json` 的 `impact` 部分配置：

```json
{
  "impact": {
    "enabled": true,
    "analysis_interval": 5,
    "cpu_threshold": 80,
    "memory_threshold": 85,
    "disk_io_threshold": 100,
    "proc_cpu_threshold": 50,
    "proc_memory_threshold": 1000,
    "proc_threads_threshold": 500,
    "proc_fds_threshold": 1000
  }
}
```

---

## 日志系统

### 日志格式

所有日志统一使用 JSONL 格式，每行一个 JSON 对象：

```json
{"timestamp":"2026-01-23T10:30:00Z","level":"INFO","category":"METRIC","message":"Process metrics collected","data":{"pid":1234,"cpu":25.5}}
```

### 日志类别

| 类别 | 说明 |
|------|------|
| SERVICE | 服务运行日志 |
| METRIC | 指标采集日志 |
| EVENT | 事件日志（进程启动/退出） |
| IMPACT | 影响分析日志 |

### 日志文件

日志保存在 `logs/` 目录，文件名格式：`monitor_YYYYMMDD_HHMMSS.jsonl`

---

## 服务部署

### Windows 服务

以管理员身份运行 PowerShell：

```powershell
# 安装服务
.\monitor-agent.exe -install

# 启动服务
.\monitor-agent.exe -start

# 查看状态
.\monitor-agent.exe -status

# 停止服务
.\monitor-agent.exe -stop

# 卸载服务
.\monitor-agent.exe -uninstall
```

### Linux systemd

```bash
# 安装服务（需要 root）
sudo ./monitor-agent -install

# 启用并启动
sudo systemctl daemon-reload
sudo systemctl enable monitor-agent
sudo systemctl start monitor-agent

# 查看状态
sudo systemctl status monitor-agent

# 卸载
sudo ./monitor-agent -uninstall
```

---

## API 接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/processes` | GET | 获取所有进程列表 |
| `/api/system` | GET | 获取系统指标 |
| `/api/monitor/targets` | GET | 获取监控目标列表 |
| `/api/monitor/add` | POST | 添加监控目标 |
| `/api/monitor/remove` | POST | 移除监控目标 |
| `/api/monitor/removeAll` | POST | 移除所有目标 |
| `/api/monitor/update` | POST | 更新目标配置 |
| `/api/events` | GET | 获取事件日志 |
| `/api/process-changes` | GET | 获取进程变化记录 |
| `/api/impacts` | GET | 获取影响事件 |
| `/api/impact/summary` | GET | 获取影响统计 |

---

## 系统架构

```
monitor-agent/
├── cmd/web/              # 主程序入口
├── cli/                  # CLI 命令行界面
│   ├── cli.go            # CLI 主框架
│   ├── formatter.go      # 输出格式化
│   ├── cmd_config.go     # 配置命令组
│   ├── cmd_target.go     # 目标命令组
│   ├── cmd_impact.go     # 影响命令组
│   ├── cmd_system.go     # 系统命令组
│   └── cmd_log.go        # 日志命令组
├── monitor/              # 监控核心逻辑
│   ├── multi_monitor.go  # 多进程监控器
│   └── process_tracker.go # 进程变化追踪器
├── impact/               # 影响分析
│   ├── analyzer.go       # 影响分析器
│   ├── file_checker.go   # 文件冲突检测
│   └── port_checker.go   # 端口冲突检测
├── provider/             # 系统指标采集
├── netmon/               # 网络流量监控
├── server/               # HTTP 服务
├── service/              # 系统服务支持
├── logger/               # 统一日志
├── buffer/               # 数据结构
├── config/               # 配置管理
├── types/                # 类型定义
└── logs/                 # 日志输出目录
```

---

## 常见问题

### Q: 网络流量显示为 0？
A: 需要管理员/root 权限才能进行网络抓包。Windows 需要安装 Npcap，Linux 需要 libpcap-dev。

### Q: CPU IO等待在 Windows 上显示为 0？
A: 正常现象，Windows 不提供 IO 等待时间指标。

### Q: 如何只使用 CLI 不启动 Web？
A: 使用 `-cli-only` 参数，或在配置中设置 `server.enabled = false`。

### Q: CLI 和 Web 数据是否同步？
A: 是的，两者共享同一个监控实例，数据完全同步。

### Q: 如何监控远程服务器？
A: 在远程服务器部署本程序，通过 `http://<服务器IP>:8080` 访问。

---

## 许可证

MIT License
