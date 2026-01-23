package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogEntry 统一日志条目
type LogEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Level     string      `json:"level"`     // INFO, WARN, ERROR, DEBUG
	Category  string      `json:"category"`  // SERVICE, EVENT, IMPACT, METRIC
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"` // 可选的附加数据
}

// Logger 统一日志器
type Logger struct {
	mu            sync.Mutex
	logFile       *os.File
	logDir        string
	consoleOutput bool
	fileOutput    bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init 初始化全局日志器
func Init(logDir string, fileOutput, consoleOutput bool) error {
	var initErr error
	once.Do(func() {
		logger, err := NewLogger(logDir, fileOutput, consoleOutput)
		if err != nil {
			initErr = err
			return
		}
		defaultLogger = logger
	})
	return initErr
}

// NewLogger 创建新的日志器
func NewLogger(logDir string, fileOutput, consoleOutput bool) (*Logger, error) {
	if logDir == "" {
		exe, _ := os.Executable()
		logDir = filepath.Join(filepath.Dir(exe), "logs")
	}
	os.MkdirAll(logDir, 0755)

	l := &Logger{
		logDir:        logDir,
		fileOutput:    fileOutput,
		consoleOutput: consoleOutput,
	}

	if fileOutput {
		if err := l.openLogFile(); err != nil {
			return nil, err
		}
	}

	return l, nil
}

// openLogFile 打开或创建日志文件
func (l *Logger) openLogFile() error {
	logPath := filepath.Join(l.logDir, fmt.Sprintf("monitor_%s.jsonl", time.Now().Format("20060102_150405")))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	l.logFile = f
	return nil
}

// Close 关闭日志器
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}
}

// Reopen 重新打开日志文件（用于日志轮转或重启后）
func (l *Logger) Reopen() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logFile != nil {
		l.logFile.Close()
	}
	return l.openLogFile()
}

// Log 写入日志
func (l *Logger) Log(level, category, message string, data interface{}) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Message:   message,
		Data:      data,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 输出到文件
	if l.fileOutput && l.logFile != nil {
		jsonData, err := json.Marshal(entry)
		if err == nil {
			l.logFile.Write(append(jsonData, '\n'))
		}
	}

	// 输出到控制台
	if l.consoleOutput {
		fmt.Printf("%s [%s] [%s] %s\n",
			entry.Timestamp.Format("2006/01/02 15:04:05"),
			level, category, message)
	}
}

// LogData 写入带有数据的日志（数据直接作为JSON输出）
func (l *Logger) LogData(category string, data interface{}) {
	// 对于纯数据日志，包装成带时间戳和类别的格式
	entry := struct {
		Timestamp time.Time   `json:"timestamp"`
		Category  string      `json:"category"`
		Data      interface{} `json:"data"`
	}{
		Timestamp: time.Now(),
		Category:  category,
		Data:      data,
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileOutput && l.logFile != nil {
		jsonData, err := json.Marshal(entry)
		if err == nil {
			l.logFile.Write(append(jsonData, '\n'))
		}
	}
}

// Info 输出 INFO 级别日志
func (l *Logger) Info(category, message string) {
	l.Log("INFO", category, message, nil)
}

// Infof 输出格式化的 INFO 级别日志
func (l *Logger) Infof(category, format string, args ...interface{}) {
	l.Log("INFO", category, fmt.Sprintf(format, args...), nil)
}

// Warn 输出 WARN 级别日志
func (l *Logger) Warn(category, message string) {
	l.Log("WARN", category, message, nil)
}

// Warnf 输出格式化的 WARN 级别日志
func (l *Logger) Warnf(category, format string, args ...interface{}) {
	l.Log("WARN", category, fmt.Sprintf(format, args...), nil)
}

// Error 输出 ERROR 级别日志
func (l *Logger) Error(category, message string) {
	l.Log("ERROR", category, message, nil)
}

// Errorf 输出格式化的 ERROR 级别日志
func (l *Logger) Errorf(category, format string, args ...interface{}) {
	l.Log("ERROR", category, fmt.Sprintf(format, args...), nil)
}

// Event 输出事件日志
func (l *Logger) Event(eventType string, pid int32, name, message string) {
	l.Log("INFO", "EVENT", fmt.Sprintf("%s: %s (pid=%d, name=%s)", eventType, message, pid, name), map[string]interface{}{
		"event_type": eventType,
		"pid":        pid,
		"name":       name,
	})
}

// Impact 输出影响分析日志
func (l *Logger) Impact(impactType, severity, target, source, detail string) {
	l.Log("INFO", "IMPACT", fmt.Sprintf("[%s] [%s] 目标: %s, 来源: %s - %s", impactType, severity, target, source, detail), map[string]interface{}{
		"impact_type": impactType,
		"severity":    severity,
		"target":      target,
		"source":      source,
	})
}

// Metric 输出指标数据
func (l *Logger) Metric(data interface{}) {
	l.LogData("METRIC", data)
}

// GetLogDir 获取日志目录
func (l *Logger) GetLogDir() string {
	return l.logDir
}

// GetWriter 获取日志写入器（用于兼容标准log包）
func (l *Logger) GetWriter() io.Writer {
	return &logWriter{logger: l}
}

// logWriter 实现 io.Writer 接口，用于兼容标准log包
type logWriter struct {
	logger *Logger
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// 移除末尾换行符
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	w.logger.Info("LOG", msg)
	return len(p), nil
}

// 全局函数，使用默认日志器

// Default 获取默认日志器
func Default() *Logger {
	return defaultLogger
}

// Info 全局 Info
func Info(category, message string) {
	if defaultLogger != nil {
		defaultLogger.Info(category, message)
	}
}

// Infof 全局 Infof
func Infof(category, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Infof(category, format, args...)
	}
}

// Warn 全局 Warn
func Warn(category, message string) {
	if defaultLogger != nil {
		defaultLogger.Warn(category, message)
	}
}

// Warnf 全局 Warnf
func Warnf(category, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warnf(category, format, args...)
	}
}

// Error 全局 Error
func Error(category, message string) {
	if defaultLogger != nil {
		defaultLogger.Error(category, message)
	}
}

// Errorf 全局 Errorf
func Errorf(category, format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Errorf(category, format, args...)
	}
}

// Event 全局 Event
func Event(eventType string, pid int32, name, message string) {
	if defaultLogger != nil {
		defaultLogger.Event(eventType, pid, name, message)
	}
}

// Impact 全局 Impact
func Impact(impactType, severity, target, source, detail string) {
	if defaultLogger != nil {
		defaultLogger.Impact(impactType, severity, target, source, detail)
	}
}

// Metric 全局 Metric
func Metric(data interface{}) {
	if defaultLogger != nil {
		defaultLogger.Metric(data)
	}
}

// Close 关闭默认日志器
func Close() {
	if defaultLogger != nil {
		defaultLogger.Close()
	}
}
