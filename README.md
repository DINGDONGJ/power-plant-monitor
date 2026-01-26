# 电厂核心软件监视保障系统

专为电厂 DCS、SIS、MIS 等核心软件系统设计的运行状态监控工具。

## 产品特点

- **专业定位**：专为电厂控制系统、厂级监控系统（SIS）、管理信息系统（MIS）等核心软件设计
- **安全合规**：满足电力二次系统安全防护要求，支持独立部署
- **无人值守**：支持 7×24 小时系统服务模式，适配电厂值班运维模式
- **事件追溯**：自动记录软件运行事件，便于缺陷追溯和故障分析
- **双平台支持**：Windows / Linux 双平台，适配各类厂站服务器环境

## 适用场景

| 系统层级 | 典型软件 | 监控价值 |
|----------|----------|----------|
| 控制层 | DCS 操作员站、工程师站、DEH 控制软件 | 保障机组安全运行 |
| 监视层 | SIS 实时数据库、Web 发布服务、历史数据库 | 确保数据采集连续性 |
| 管理层 | MIS 应用服务、OA 系统、设备管理系统 | 保障日常管理业务 |
| 接口层 | OPC 服务、调度通信网关、远动终端 | 确保数据交换正常 |
| 安全层 | 安全监控代理、日志审计、防病毒软件 | 维护信息安全体系 |

## 核心功能

### 软件状态监控
- 实时监控厂站核心软件的运行状态
- 采集 CPU、内存、磁盘 IO、网络流量等性能指标
- 检测软件异常退出、重启等状态变化
- 自动记录软件运行事件日志

### 风险关联分析
- 分析其他软件对保障对象的潜在影响
- 检测资源竞争（CPU/内存/磁盘/网络）
- 检测端口冲突和文件占用
- 提供处理建议，辅助运维决策

### 运维支持
- Web 界面远程查看，支持值班室大屏展示
- CLI 命令行管理，适合服务器环境操作
- 日志导出功能，支持生成值班运行报告
- 系统服务模式，实现无人值守运行

---

## 快速开始

### 环境要求
- Go 1.19 或更高版本
- Windows 7+ 或 Linux（支持 systemd）
- 管理员/root 权限（用于网络包捕获）

### 编译依赖

软件网络流量监控使用 gopacket 抓包，需要安装 pcap 库：

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
go build -o monitor-web.exe ./cmd/web

# 编译 Linux 版本
GOOS=linux go build -o monitor-web ./cmd/web
```

### 配置

生成配置文件：
```bash
./monitor-web.exe -gen-config
```

编辑 `config.json`（电厂典型配置示例）：
```json
{
  "server": {
    "addr": ":8080",
    "enabled": true
  },
  "targets": [
    {
      "pid": 0,
      "name": "edpf_hmi.exe",
      "alias": "DCS操作员站"
    },
    {
      "pid": 0,
      "name": "historian.exe",
      "alias": "SIS历史数据库"
    },
    {
      "pid": 0,
      "name": "opcserver.exe",
      "alias": "OPC数据服务"
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

### 模式一：Web UI 模式（推荐值班室使用）

```bash
./monitor-web.exe -config config.json
```

- 启动 Web 服务器，访问 `http://localhost:8080` 查看监控界面
- 适合远程监控和值班室大屏展示
- 按 `Ctrl+C` 退出

### 模式二：CLI + Web 同时运行

```bash
./monitor-web.exe -cli -config config.json
```

- CLI 在前台运行，可交互操作
- Web 服务器在后台运行
- **两者共享同一个监控实例，数据完全同步**
- 在 CLI 中的操作，Web 界面立即可见

### 模式三：纯 CLI 模式

```bash
./monitor-web.exe -cli-only -config config.json
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
- `proc-cpu` - 软件 CPU 阈值（%）
- `proc-mem` - 软件内存阈值（MB）

### 保障对象管理 (target)

| 命令 | 说明 | 示例 |
|------|------|------|
| `target list` | 列出所有保障对象 | `target list` |
| `target add <pid\|name> [alias]` | 添加保障对象 | `target add edpf_hmi.exe DCS操作员站` |
| `target remove <pid>` | 解除保障对象 | `target remove 1234` |
| `target info <pid>` | 显示对象详情 | `target info 1234` |
| `target update <pid> <key> <val>` | 更新对象配置 | `target update 1234 alias DCS工程师站` |
| `target clear` | 清除所有对象 | `target clear` |

### 风险分析 (impact)

| 命令 | 说明 |
|------|------|
| `impact list [n]` | 显示风险事件（默认20条） |
| `impact summary` | 显示风险统计汇总 |
| `impact config` | 显示风险分析配置 |
| `impact set <key> <value>` | 设置风险分析参数 |
| `impact clear` | 清除所有风险事件 |

### 系统信息 (system)

| 命令 | 说明 | 示例 |
|------|------|------|
| `system status` | 显示系统整体状态 | `system status` |
| `system top [n]` | 显示 Top N 软件（按 CPU） | `system top 20` |
| `system ps [pattern]` | 列出软件（可过滤） | `system ps dcs` |
| `system events [n]` | 显示最近事件 | `system events 50` |
| `system watch <pid>` | 实时监控软件（60秒） | `system watch 1234` |

### 日志管理 (log)

| 命令 | 说明 |
|------|------|
| `log tail [n]` | 查看最近 N 条日志（默认50） |
| `log filter <type>` | 按类型过滤（METRIC/EVENT/IMPACT） |
| `log export <file>` | 导出日志到文件 |
| `log report <file>` | 生成值班运行报告 |
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
- 厂站核心软件保障清单，显示关键指标和状态
- 风险事件实时显示

### 厂站软件列表
- 实时显示系统所有软件
- 支持按名称、PID、用户搜索
- 同名软件自动分组
- 可自定义显示列，拖拽调整顺序

### 保障管理
- 点击软件即可纳入保障
- 支持设置备注名称（如"DCS操作员站"）
- 实时显示保障对象的性能指标

### 风险关联分析
- 自动检测对保障对象的潜在影响
- 显示风险类型、严重程度、来源软件
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

## 风险关联分析

### 检测类型

| 类型 | 说明 |
|------|------|
| CPU 竞争 | 其他软件占用大量 CPU，影响保障对象 |
| 内存压力 | 系统内存不足或其他软件内存占用过高 |
| 磁盘 IO | 其他软件磁盘读写影响保障对象 |
| 网络 IO | 其他软件网络流量影响保障对象 |
| 端口冲突 | 其他软件占用保障对象的端口 |
| 文件冲突 | 其他软件访问保障对象的关键文件 |

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
{"timestamp":"2026-01-23T10:30:00Z","level":"INFO","category":"METRIC","message":"Software metrics collected","data":{"pid":1234,"cpu":25.5}}
```

### 日志类别

| 类别 | 说明 |
|------|------|
| SERVICE | 服务运行日志 |
| METRIC | 指标采集日志 |
| EVENT | 事件日志（软件启动/退出） |
| IMPACT | 风险分析日志 |

### 日志文件

日志保存在 `logs/` 目录，文件名格式：`monitor_YYYYMMDD_HHMMSS.jsonl`

### 值班运行报告

使用 `log report` 命令可生成电厂风格的值班运行报告：

```
═══════════════════════════════════════════════════════════════
              电厂核心软件运行日报
═══════════════════════════════════════════════════════════════
单位名称：XX发电厂
报告日期：2026-01-26
值    次：白班 (08:00 - 20:00)
生成时间：2026-01-26 20:00:00
───────────────────────────────────────────────────────────────

一、保障软件运行情况
  序号  软件名称              状态    CPU均值  内存均值  运行时长
  1     DCS操作员站           正常    2.5%     256MB     12小时
  2     SIS历史数据库         正常    5.2%     1.2GB     12小时
  3     OPC数据服务           正常    1.8%     128MB     12小时

二、运行事件统计
  软件启动：2 次
  软件退出：0 次
  异常告警：0 次

三、风险事件统计
  严重：0    高级：0    中级：2    低级：5

四、详细事件记录
  [10:30:25] [中级] CPU竞争 - Windows Update 占用 CPU 45%
  [14:22:10] [中级] 内存压力 - 系统可用内存低于 15%

五、值班备注
  （无）

───────────────────────────────────────────────────────────────
                    值班员签名：___________
═══════════════════════════════════════════════════════════════
```

---

## 服务部署

### Windows 服务

以管理员身份运行 PowerShell：

```powershell
# 安装服务
.\monitor-web.exe -install

# 启动服务
.\monitor-web.exe -start

# 查看状态
.\monitor-web.exe -status

# 停止服务
.\monitor-web.exe -stop

# 卸载服务
.\monitor-web.exe -uninstall
```

### Linux systemd

```bash
# 安装服务（需要 root）
sudo ./monitor-web -install

# 启用并启动
sudo systemctl daemon-reload
sudo systemctl enable monitor-agent
sudo systemctl start monitor-agent

# 查看状态
sudo systemctl status monitor-agent

# 卸载
sudo ./monitor-web -uninstall
```

---

## API 接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/processes` | GET | 获取所有软件列表 |
| `/api/system` | GET | 获取系统指标 |
| `/api/monitor/targets` | GET | 获取保障对象列表 |
| `/api/monitor/add` | POST | 添加保障对象 |
| `/api/monitor/remove` | POST | 解除保障对象 |
| `/api/monitor/removeAll` | POST | 解除所有对象 |
| `/api/monitor/update` | POST | 更新对象配置 |
| `/api/events` | GET | 获取事件日志 |
| `/api/process-changes` | GET | 获取软件变化记录 |
| `/api/impacts` | GET | 获取风险事件 |
| `/api/impact/summary` | GET | 获取风险统计 |

---

## 系统架构

```
monitor-agent/
├── cmd/web/              # 主程序入口
├── cli/                  # CLI 命令行界面
│   ├── cli.go            # CLI 主框架
│   ├── formatter.go      # 输出格式化
│   ├── cmd_config.go     # 配置命令组
│   ├── cmd_target.go     # 保障对象命令组
│   ├── cmd_impact.go     # 风险分析命令组
│   ├── cmd_system.go     # 系统命令组
│   └── cmd_log.go        # 日志命令组
├── monitor/              # 监控核心逻辑
│   ├── multi_monitor.go  # 多软件监控器
│   └── process_tracker.go # 软件变化追踪器
├── impact/               # 风险分析
│   ├── analyzer.go       # 风险分析器
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

### Q: 如何与现有 DCS/SIS 系统集成？
A: 本系统独立运行，不侵入现有系统，只通过操作系统层面监控软件运行状态。

---

## 许可证

MIT License
