# 电厂核心软件监视保障系统

一个轻量级的进程监控代理系统，提供实时进程监控和 Web 管理界面。支持 Windows 和 Linux 双平台，可部署为系统服务实现 7×24 小时无人值守运行。

## 项目背景

在电厂等关键基础设施环境中，核心软件的稳定运行至关重要。本系统旨在：
- 实时监控关键进程的运行状态
- 采集系统和进程级别的详细性能指标
- 检测进程启动/退出等状态变化
- 提供直观的 Web 界面进行管理和查看

## 功能特性

### 进程监控
- 实时采集系统所有进程的 CPU、内存、磁盘 IO、网络流量等指标
- 支持按进程名、PID、用户等条件搜索过滤
- 同名进程自动分组，支持展开/折叠查看
- 可自定义显示列，支持拖拽调整列顺序和宽度
- 自动检测新进程启动和进程消失

### Web 界面
- 终端风格的黑绿配色，专业感强
- 系统资源实时曲线图（CPU/内存/网络），60秒历史数据
- 监控列表和进程列表双表格布局
- 响应式设计，支持各种屏幕尺寸

## 监控指标说明

### 系统级指标

| 指标 | JSON 字段 | 数据来源 | 说明 |
|------|-----------|----------|------|
| CPU 总使用率 | `cpu_percent` | gopsutil `cpu.Times()` | 系统整体 CPU 占用百分比 |
| CPU 用户态 | `cpu_user` | gopsutil `cpu.Times()` | 用户态 CPU 占用百分比 |
| CPU 内核态 | `cpu_system` | gopsutil `cpu.Times()` | 内核态 CPU 占用百分比 |
| CPU IO等待 | `cpu_iowait` | gopsutil `cpu.Times()` | 等待 IO 的 CPU 百分比（Linux 特有） |
| CPU 空闲 | `cpu_idle` | gopsutil `cpu.Times()` | 空闲 CPU 百分比 |
| 内存总量 | `memory_total` | gopsutil `mem.VirtualMemory()` | 系统物理内存总量 |
| 内存已用 | `memory_used` | gopsutil `mem.VirtualMemory()` | 已使用的物理内存 |
| 内存可用 | `memory_available` | gopsutil `mem.VirtualMemory()` | 可用内存（含缓存可回收部分） |
| 内存使用率 | `memory_percent` | 计算值 | 已用内存 / 总内存 × 100% |
| Swap 总量 | `swap_total` | gopsutil `mem.SwapMemory()` | 交换空间总量 |
| Swap 已用 | `swap_used` | gopsutil `mem.SwapMemory()` | 已使用的交换空间 |
| Swap 换入速率 | `swap_in_rate` | gopsutil `mem.SwapMemory()` | 每秒换入字节数 |
| Swap 换出速率 | `swap_out_rate` | gopsutil `mem.SwapMemory()` | 每秒换出字节数 |
| 网络接收速率 | `net_recv_rate` | gopacket 抓包统计 | 所有进程接收流量之和 (B/s) |
| 网络发送速率 | `net_send_rate` | gopacket 抓包统计 | 所有进程发送流量之和 (B/s) |
| 磁盘读取速率 | `disk_read_rate` | gopsutil `disk.IOCounters()` | 系统磁盘读取速率 (B/s) |
| 磁盘写入速率 | `disk_write_rate` | gopsutil `disk.IOCounters()` | 系统磁盘写入速率 (B/s) |
| 磁盘读取 IOPS | `disk_read_ops` | gopsutil `disk.IOCounters()` | 每秒磁盘读取操作数 |
| 磁盘写入 IOPS | `disk_write_ops` | gopsutil `disk.IOCounters()` | 每秒磁盘写入操作数 |

### 进程级指标

| 指标 | 列名 | JSON 字段 | 数据来源 | 说明 |
|------|------|-----------|----------|------|
| 进程名称 | 进程名称 | `name` | gopsutil `proc.Name()` | 可执行文件名 |
| 进程 ID | PID | `pid` | gopsutil `proc.Pid` | 系统分配的进程标识符 |
| 运行状态 | 状态 | `status` | gopsutil `proc.Status()` | running/sleeping/stopped 等 |
| 用户名 | 发布者 | `username` | gopsutil `proc.Username()` | 进程所属用户 |
| CPU 占用 | CPU% | `cpu_pct` | gopsutil `proc.CPUPercent()` | 进程 CPU 使用百分比 |
| 物理内存 | 内存 | `rss_bytes` | gopsutil `proc.MemoryInfo().RSS` | 常驻内存集大小 |
| 内存增长速率 | 内存增速 | `rss_growth_rate` | 计算值 | RSS 每秒变化量 (B/s)，正值=增长，负值=释放 |
| 虚拟内存 | 虚拟内存 | `vms` | gopsutil `proc.MemoryInfo().VMS` | 虚拟内存大小 |
| 页面缓冲池 | 页面池 | `paged_pool` | 平台特定 | 可换出到磁盘的内存 |
| 非页面缓冲池 | 非页面池 | `non_paged_pool` | 平台特定 | 必须常驻物理内存的数据 |
| 句柄数 | 句柄 | `num_fds` | 平台特定 | 打开的文件/资源句柄数 |
| 磁盘读取速率 | 磁盘读 | `disk_read_rate` | gopsutil `proc.IOCounters()` | 每秒读取字节数 |
| 磁盘写入速率 | 磁盘写 | `disk_write_rate` | gopsutil `proc.IOCounters()` | 每秒写入字节数 |
| 网络接收速率 | 网络收 | `net_recv_rate` | gopacket 抓包 + 端口映射 | 进程每秒接收字节数 |
| 网络发送速率 | 网络发 | `net_send_rate` | gopacket 抓包 + 端口映射 | 进程每秒发送字节数 |
| 运行时长 | 已运行 | `uptime` | gopsutil `proc.CreateTime()` | 当前时间 - 进程创建时间（秒） |
| 命令行 | 命令行 | `cmdline` | gopsutil `proc.Cmdline()` | 完整启动命令 |

### 进程状态变化检测

系统自动检测并记录以下进程变化事件：

| 事件类型 | JSON `type` | 说明 |
|----------|-------------|------|
| 新进程启动 | `new_process` | 检测到系统中出现新进程 |
| 进程消失 | `process_gone` | 检测到进程从系统中消失 |
| 监控目标退出 | `exit` | 被监控的目标进程退出 |

### 网络流量监控原理

进程网络流量使用 **gopacket 抓包** 实现精确统计：

1. **抓包**：在所有非 loopback 网卡上启动抓包，过滤 TCP/UDP 流量
2. **端口映射**：每 2 秒通过 `net.Connections()` 更新端口到 PID 的映射表
3. **流量归属**：
   - 源端口匹配本机进程 → 计入该进程的发送流量
   - 目标端口匹配本机进程 → 计入该进程的接收流量
4. **速率计算**：每秒计算一次流量差值得到速率

**特点**：
- 系统总流量 = 所有进程流量之和（数据源统一）
- 精确到每个数据包
- 需要管理员/root 权限

### 平台差异

| 指标 | Windows | Linux |
|------|---------|-------|
| CPU IO等待 | 不支持（返回 0） | 支持 |
| 句柄数 | `GetProcessHandleCount` Win32 API | `/proc/[pid]/fd` 目录计数 |
| 页面缓冲池 | `PROCESS_MEMORY_COUNTERS_EX.QuotaPagedPoolUsage` | `memInfo.Swap` 近似 |
| 非页面缓冲池 | `PROCESS_MEMORY_COUNTERS_EX.QuotaNonPagedPoolUsage` | `memInfo.Data` 近似 |
| 网络抓包 | 需要 Npcap | 需要 libpcap-dev |

### 数据采集频率

| 数据类型 | 后端采集间隔 | 前端刷新间隔 |
|----------|--------------|--------------|
| 系统指标 | 1 秒 | 2 秒 |
| 进程列表 | 按需（API 调用时） | 2 秒 |
| 监控目标指标 | 1 秒 | 2 秒 |
| 端口-PID 映射 | 2 秒 | - |
| 网络流量速率 | 1 秒 | 2 秒 |

## 系统架构

```
monitor-agent/
├── cmd/web/              # 主程序入口
│   ├── main.go           # 命令行参数处理
│   ├── signal_windows.go # Windows 信号处理
│   └── signal_linux.go   # Linux 信号处理
├── monitor/              # 监控核心逻辑
│   ├── multi_monitor.go  # 多进程监控器
│   └── process_tracker.go # 进程变化追踪器
├── provider/             # 系统指标采集
│   ├── provider.go       # 接口定义
│   ├── provider_common.go  # 通用实现（gopsutil）
│   ├── provider_windows.go # Windows 特定实现
│   └── provider_linux.go   # Linux 特定实现
├── netmon/               # 网络流量监控
│   └── netmon.go         # gopacket 抓包实现
├── server/               # HTTP 服务
│   ├── web_server.go     # API 路由和处理
│   └── static/           # 静态资源（嵌入到二进制）
│       └── index.html    # Web 界面
├── service/              # 系统服务支持
│   ├── service.go        # 服务核心逻辑
│   ├── service_windows.go # Windows Service 实现
│   └── service_linux.go    # Linux systemd 实现
├── buffer/               # 数据结构
│   └── ring.go           # 泛型环形缓冲区
├── logger/               # 日志记录
│   └── jsonl.go          # JSONL 格式日志
├── types/                # 数据类型定义
│   └── types.go          # 结构体定义
└── logs/                 # 日志输出目录
```


## 技术栈

### 后端语言
- **Go 1.19+**：高性能、跨平台编译、原生并发支持、单文件部署

### 核心依赖

| 包名 | 版本 | 用途 |
|------|------|------|
| `github.com/shirou/gopsutil/v3` | v3.23.12 | 跨平台系统信息采集（进程、CPU、内存、磁盘 IO） |
| `github.com/google/gopacket` | v1.1.19 | 网络抓包，精确统计进程网络流量 |
| `golang.org/x/sys` | v0.15.0 | Windows Service API 支持 |

### 编译依赖

进程网络流量监控使用 gopacket 抓包，需要安装 pcap 库：

**Linux**：
```bash
sudo apt install libpcap-dev
```

**Windows**：
- 下载安装 [Npcap](https://npcap.com/)
- 安装时勾选 "Install Npcap in WinPcap API-compatible Mode"

### 前端技术
- **HTML5/CSS3**：页面结构和样式，终端风格设计
- **原生 JavaScript (ES6+)**：无框架依赖，轻量高效
- **Canvas API**：实时绘制 CPU/内存时间序列曲线图
- **Fetch API**：异步请求后端 RESTful API
- **LocalStorage**：保存用户列配置偏好

### 数据格式
- **JSON**：API 请求响应格式
- **JSONL**：日志文件格式（每行一个 JSON 对象，便于流式处理）

## 快速开始

### 环境要求
- Go 1.19 或更高版本
- Windows 7+ 或 Linux（支持 systemd）

### 编译

```bash
# 克隆项目
git clone <repository-url>
cd monitor-agent

# 编译 Windows 版本（自动请求管理员权限）
go build -o monitor-web.exe ./cmd/web

# 编译 Linux 版本
GOOS=linux go build -o monitor-web ./cmd/web
```

### 运行

```bash
# Windows（双击或命令行运行，会自动请求管理员权限）
.\monitor-web.exe

# Linux（需要 root 权限以支持网络抓包）
sudo ./monitor-web
```

启动后访问 http://localhost:8080

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-addr` | HTTP 服务监听地址 | `:8080` |
| `-log-dir` | 日志文件目录 | `./logs` |
| `-service` | 以服务模式运行 | `false` |
| `-install` | 安装为系统服务 | - |
| `-uninstall` | 卸载系统服务 | - |
| `-start` | 启动服务 | - |
| `-stop` | 停止服务 | - |
| `-status` | 查看服务状态 | - |
| `-version` | 显示版本号 | - |

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
# 安装服务（需要 root 权限）
sudo ./monitor-web -install

# 启用并启动
sudo systemctl daemon-reload
sudo systemctl enable monitor-agent
sudo systemctl start monitor-agent

# 查看状态
sudo systemctl status monitor-agent

# 卸载服务
sudo ./monitor-web -uninstall
```

## API 接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/processes` | GET | 获取所有进程列表 |
| `/api/system` | GET | 获取系统指标（CPU/内存/磁盘/网络） |
| `/api/monitor/targets` | GET | 获取监控目标列表 |
| `/api/monitor/add` | POST | 添加监控目标 |
| `/api/monitor/remove` | POST | 移除监控目标 |
| `/api/monitor/removeAll` | POST | 移除所有目标 |
| `/api/monitor/update` | POST | 更新目标配置 |
| `/api/events` | GET | 获取事件日志 |
| `/api/process-changes` | GET | 获取进程变化记录 |
| `/api/status` | GET | 获取监控状态 |

### API 响应示例

**GET /api/system**
```json
{
  "cpu_percent": 25.5,
  "cpu_user": 15.2,
  "cpu_system": 8.3,
  "cpu_iowait": 2.0,
  "cpu_idle": 74.5,
  "memory_total": 17179869184,
  "memory_used": 8589934592,
  "memory_available": 8589934592,
  "memory_percent": 50.0,
  "swap_total": 4294967296,
  "swap_used": 1073741824,
  "swap_percent": 25.0,
  "swap_in_rate": 0,
  "swap_out_rate": 0,
  "net_recv_rate": 102400,
  "net_send_rate": 51200,
  "disk_read_rate": 1048576,
  "disk_write_rate": 524288,
  "disk_read_ops": 100,
  "disk_write_ops": 50
}
```

**GET /api/process-changes**
```json
[
  {
    "timestamp": "2026-01-16T10:30:00Z",
    "type": "new",
    "pid": 12345,
    "name": "notepad.exe",
    "cmdline": "C:\\Windows\\notepad.exe"
  },
  {
    "timestamp": "2026-01-16T10:29:00Z",
    "type": "gone",
    "pid": 12340,
    "name": "calc.exe"
  }
]
```

## 日志文件

日志保存在 `logs/` 目录：

| 文件 | 说明 |
|------|------|
| `service.log` | 服务运行日志 |
| `multi_monitor_*.jsonl` | 监控数据（JSONL 格式） |

## 常见问题

### Q: 网络流量显示为 0？
A: 需要管理员/root 权限才能进行网络抓包。Windows 需要安装 Npcap，Linux 需要 libpcap-dev。

### Q: CPU IO等待在 Windows 上显示为 0？
A: 这是正常的，Windows 不提供 IO 等待时间指标。

### Q: 内存增速列显示红色/绿色是什么意思？
A: 红色表示内存正在增长（可能存在内存泄漏），绿色表示内存正在释放，灰色表示稳定。

### Q: 如何监控远程服务器？
A: 在远程服务器上部署本程序，通过 `http://<服务器IP>:8080` 访问。注意配置防火墙规则。

## 许可证

MIT License
