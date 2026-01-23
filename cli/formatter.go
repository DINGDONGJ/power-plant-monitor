package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Color constants for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// Formatter 输出格式化器
type Formatter struct {
	colorEnabled bool
}

// NewFormatter 创建格式化器
func NewFormatter() *Formatter {
	return &Formatter{
		colorEnabled: true, // Windows 10+ 和大部分终端支持 ANSI 颜色
	}
}

// Color 添加颜色
func (f *Formatter) Color(color, text string) string {
	if !f.colorEnabled {
		return text
	}
	return color + text + ColorReset
}

// Bold 粗体
func (f *Formatter) Bold(text string) string {
	return f.Color(ColorBold, text)
}

// Success 成功消息（绿色）
func (f *Formatter) Success(text string) string {
	return f.Color(ColorGreen, "✓ "+text)
}

// Error 错误消息（红色）
func (f *Formatter) Error(text string) string {
	return f.Color(ColorRed, "✗ "+text)
}

// Warning 警告消息（黄色）
func (f *Formatter) Warning(text string) string {
	return f.Color(ColorYellow, "⚠ "+text)
}

// Info 信息消息（蓝色）
func (f *Formatter) Info(text string) string {
	return f.Color(ColorCyan, "ℹ "+text)
}

// Header 标题
func (f *Formatter) Header(text string) string {
	return f.Color(ColorBold+ColorCyan, text)
}

// StatusOK 状态正常
func (f *Formatter) StatusOK(text string) string {
	return f.Color(ColorGreen, text)
}

// StatusError 状态异常
func (f *Formatter) StatusError(text string) string {
	return f.Color(ColorRed, text)
}

// StatusWarn 状态警告
func (f *Formatter) StatusWarn(text string) string {
	return f.Color(ColorYellow, text)
}

// Divider 分隔线
func (f *Formatter) Divider(width int) string {
	return strings.Repeat("─", width)
}

// DoubleDivider 双线分隔
func (f *Formatter) DoubleDivider(width int) string {
	return strings.Repeat("═", width)
}

// Table 创建表格输出
type Table struct {
	writer  *tabwriter.Writer
	headers []string
}

// NewTable 创建表格
func NewTable(headers ...string) *Table {
	t := &Table{
		writer:  tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0),
		headers: headers,
	}
	return t
}

// PrintHeader 打印表头
func (t *Table) PrintHeader() {
	fmt.Fprintln(t.writer, strings.Join(t.headers, "\t"))
	dividers := make([]string, len(t.headers))
	for i, h := range t.headers {
		dividers[i] = strings.Repeat("─", len(h)+2)
	}
	fmt.Fprintln(t.writer, strings.Join(dividers, "\t"))
}

// AddRow 添加行
func (t *Table) AddRow(values ...string) {
	fmt.Fprintln(t.writer, strings.Join(values, "\t"))
}

// Flush 输出表格
func (t *Table) Flush() {
	t.writer.Flush()
}

// FormatBytes 格式化字节数
func FormatBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/1024/1024/1024)
}

// FormatBytesRate 格式化字节率
func FormatBytesRate(bytesPerSec float64) string {
	return FormatBytes(uint64(bytesPerSec)) + "/s"
}

// FormatPercent 格式化百分比
func FormatPercent(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
}

// FormatMemGrowth 格式化内存增速
func FormatMemGrowth(rate float64) string {
	if rate > 0 {
		return fmt.Sprintf("+%s/s", FormatBytes(uint64(rate)))
	} else if rate < 0 {
		return fmt.Sprintf("-%s/s", FormatBytes(uint64(-rate)))
	}
	return "0"
}

// FormatUptime 格式化运行时间
func FormatUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%d秒", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%d分%d秒", seconds/60, seconds%60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%d时%d分", seconds/3600, (seconds%3600)/60)
	}
	return fmt.Sprintf("%d天%d时", seconds/86400, (seconds%86400)/3600)
}

// Truncate 截断字符串
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// Truncate 截断字符串 (Formatter 方法)
func (f *Formatter) Truncate(s string, maxLen int) string {
	return Truncate(s, maxLen)
}

// FormatBytes 格式化字节 (Formatter 方法)
func (f *Formatter) FormatBytes(bytes uint64) string {
	return FormatBytes(bytes)
}

// FormatPercent 格式化百分比 (Formatter 方法)
func (f *Formatter) FormatPercent(pct float64) string {
	return FormatPercent(pct)
}

// FormatBool 格式化布尔值
func (f *Formatter) FormatBool(b bool) string {
	if b {
		return f.Success("已启用")
	}
	return f.StatusError("已禁用")
}

// ProgressBar 生成进度条
func (f *Formatter) ProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 20
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	
	filled := int(percent / 100.0 * float64(width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	
	if percent >= 80 {
		return f.Color(ColorRed, bar)
	} else if percent >= 60 {
		return f.Color(ColorYellow, bar)
	}
	return f.Color(ColorGreen, bar)
}
